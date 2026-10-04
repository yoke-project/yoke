// Package peer reads who reached a socket: the kernel's credential for the process at the other end of
// a local socket. It secures nothing — reaching the socket was the whole of the authorisation — and only
// says who reached it.
package peer

import (
	"context"
	"errors"
	"net"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc/credentials"
	grpcpeer "google.golang.org/grpc/peer"
)

// Info is the kernel's credential for the process at the other end of a local socket.
type Info struct {
	credentials.CommonAuthInfo
	UID, GID uint32
	PID      int32
}

// AuthType names what established it.
func (Info) AuthType() string { return "peercred" }

// Credentials read the peer credential of every connection accepted on a local socket. A connection on
// anything else is refused, unless Unestablished admits it, with no credential, as a peer the listener
// cannot identify.
type Credentials struct{ Unestablished bool }

func (c Credentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	u, ok := conn.(*net.UnixConn)
	if !ok {
		if c.Unestablished {
			return conn, nil, nil
		}
		return nil, nil, errors.New("this surface is served on local sockets only")
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
	return conn, Info{CommonAuthInfo: credentials.CommonAuthInfo{SecurityLevel: credentials.NoSecurity},
		UID: cred.Uid, GID: cred.Gid, PID: cred.Pid}, nil
}

func (Credentials) ClientHandshake(context.Context, string, net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, errors.New("the surface's credentials serve, and dial nothing")
}

func (Credentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "peercred"}
}
func (c Credentials) Clone() credentials.TransportCredentials { return c }
func (Credentials) OverrideServerName(string) error           { return nil }

// HostAccounts resolves an account's number through the host's account database.
func HostAccounts(uid string) (string, error) {
	u, err := user.LookupId(uid)
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

type connKey struct{}

// ConnContext keeps, for an HTTP server, the peer credential of a connection accepted on a local socket,
// where Account finds it.
func ConnContext(ctx context.Context, c net.Conn) context.Context {
	if _, info, err := (Credentials{}).ServerHandshake(c); err == nil && info != nil {
		return context.WithValue(ctx, connKey{}, info)
	}
	return ctx
}

// Account is the account the connection of ctx came from: its name, or its number marked `uid:` where
// no name resolves; false where the connection carries no credential. An account name cannot hold a
// colon, so the mark cannot be a name.
func Account(ctx context.Context, accounts func(uid string) (string, error)) (string, bool) {
	cred, ok := ctx.Value(connKey{}).(Info)
	if !ok {
		p, found := grpcpeer.FromContext(ctx)
		if !found {
			return "", false
		}
		if cred, ok = p.AuthInfo.(Info); !ok {
			return "", false
		}
	}
	if accounts == nil {
		accounts = HostAccounts
	}
	uid := strconv.FormatUint(uint64(cred.UID), 10)
	name, err := accounts(uid)
	if err != nil || name == "" {
		return "uid:" + uid, true
	}
	return name, true
}
