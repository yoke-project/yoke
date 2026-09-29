package admin

import (
	"context"
	"errors"
	"net"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Peer is the kernel's credential for the process at the other end of a socket.
type Peer struct {
	credentials.CommonAuthInfo
	UID, GID uint32
	PID      int32
}

// AuthType names what established it.
func (Peer) AuthType() string { return "peercred" }

// peerCredentials reads the kernel's peer credential of every accepted connection. They secure nothing:
// reaching the socket was the whole of the authorisation, and this only says who reached it.
type peerCredentials struct{}

func (peerCredentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	u, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, nil, errors.New("the administrative surface is served on local sockets only")
	}
	raw, err := u.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	var cred *unix.Ucred
	var read error
	if err := raw.Control(func(fd uintptr) {
		cred, read = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return nil, nil, err
	}
	if read != nil {
		return nil, nil, read
	}
	return conn, Peer{CommonAuthInfo: credentials.CommonAuthInfo{SecurityLevel: credentials.NoSecurity},
		UID: cred.Uid, GID: cred.Gid, PID: cred.Pid}, nil
}

func (peerCredentials) ClientHandshake(context.Context, string, net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, errors.New("the surface's credentials serve, and dial nothing")
}

func (peerCredentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "peercred"}
}
func (c peerCredentials) Clone() credentials.TransportCredentials { return c }
func (peerCredentials) OverrideServerName(string) error           { return nil }

// hostAccounts resolves an account's number through the host's account database.
func hostAccounts(uid string) (string, error) {
	u, err := user.LookupId(uid)
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// actor is the operator the channel of ctx established: the account's name, or its number marked
// unresolved where no name resolves. An account name cannot hold a colon, so the mark cannot be a name.
func (s *Surface) actor(ctx context.Context) event.Actor {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return event.Actor{Class: event.ByOperator}
	}
	cred, ok := p.AuthInfo.(Peer)
	if !ok {
		return event.Actor{Class: event.ByOperator}
	}
	uid := strconv.FormatUint(uint64(cred.UID), 10)
	name, err := s.accounts(uid)
	if err != nil || name == "" {
		return event.Actor{Class: event.ByOperator, Person: "uid:" + uid}
	}
	return event.Actor{Class: event.ByOperator, Person: name}
}
