// Package interfaces is the interface surface's side of the Core: the channels a deployment declares,
// bound at step 9 by the class of their address, each answering in the projection it declares.
package interfaces

import (
	"cmp"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"google.golang.org/grpc"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/peer"
	"github.com/yoke-project/yoke/internal/gate"
)

// ceiling is the longest path a Unix domain socket may have: 108 bytes, the terminator included.
const ceiling = 107

// Bound are a deployment's channels, bound.
type Bound struct {
	addresses map[string]string
	closers   []func()
	surfaces  []*Surface
}

// Bind binds every channel declared, in the order of their names: a local socket at
// interfaces/<channel>.sock under the instance root, given the mode; a loopback channel on its port on
// the loopback address. A routable channel is refused, since this Core carries neither the authenticated
// transport nor the credential such a channel requires. Binding is all or nothing: on any failure what
// was bound is unbound, and the error names the channel.
func Bind(root string, mode os.FileMode, channels []gate.Channel) (*Bound, error) {
	return BindServing(root, mode, channels, nil)
}

// BindServing is Bind, each channel carrying the local projection served by the surface the function
// gives for it; with none, a local channel answers as the interface service and serves no operation.
func BindServing(root string, mode os.FileMode, channels []gate.Channel, surface func(gate.Channel) interfacev1.InterfaceServer) (*Bound, error) {
	if surface == nil {
		surface = func(ch gate.Channel) interfacev1.InterfaceServer { return NewSurface(Config{Channel: ch}) }
	}
	b := &Bound{addresses: map[string]string{}}
	ordered := slices.Clone(channels)
	slices.SortFunc(ordered, func(x, y gate.Channel) int { return cmp.Compare(x.Name, y.Name) })
	for _, ch := range ordered {
		listener, address, err := listen(root, mode, ch)
		if err != nil {
			b.Close()
			return nil, fmt.Errorf("the channel %s: %w", ch.Name, err)
		}
		b.addresses[ch.Name] = address
		served := surface(ch)
		if s, ok := served.(*Surface); ok {
			b.surfaces = append(b.surfaces, s)
		}
		b.closers = append(b.closers, serve(ch, listener, served))
	}
	return b, nil
}

// listen binds one channel by the class of its address.
func listen(root string, mode os.FileMode, ch gate.Channel) (net.Listener, string, error) {
	if ch.Address == nil || ch.Address.Class == "local" {
		path := filepath.Join(root, "interfaces", ch.Name+".sock")
		if len(path) > ceiling {
			return nil, "", fmt.Errorf("the socket path %s is %d characters, beyond the %d a socket path may have", path, len(path), ceiling)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, "", err
		}
		os.Remove(path)
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			return nil, "", err
		}
		// The socket is removed when the channel is unbound, and never by a listener closing late, which
		// would remove a socket bound since at the same path.
		listener.SetUnlinkOnClose(false)
		if err := os.Chmod(path, mode); err != nil {
			listener.Close()
			return nil, "", err
		}
		return listener, path, nil
	}
	switch ch.Address.Class {
	case "loopback":
		address := net.JoinHostPort("127.0.0.1", ch.Address.Port)
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return nil, "", err
		}
		return listener, address, nil
	case "routable":
		return nil, "", errors.New("it is on a routable address, and this Core does not yet carry the authenticated transport and the credential such a channel requires")
	}
	return nil, "", fmt.Errorf("the address class %q is not one a channel is bound by", ch.Address.Class)
}

// serve starts the channel's terminator in the projection it declares, and returns what stops it.
func serve(ch gate.Channel, listener net.Listener, local interfacev1.InterfaceServer) func() {
	if ch.Transport == "http+ws" {
		var handler http.Handler = http.HandlerFunc(http.NotFound)
		if h, ok := local.(http.Handler); ok {
			handler = h
		}
		server := &http.Server{Handler: handler, ConnContext: peer.ConnContext}
		go server.Serve(listener)
		return func() { server.Close() }
	}
	server := grpc.NewServer(grpc.Creds(peer.Credentials{Unestablished: true}))
	interfacev1.RegisterInterfaceServer(server, local)
	go server.Serve(listener)
	return func() {
		server.Stop()
		if unix, ok := listener.Addr().(*net.UnixAddr); ok {
			os.Remove(unix.Name)
		}
	}
}

// Address is where a channel is bound: its socket's path, or its host and port; empty for a channel not
// bound.
func (b *Bound) Address(name string) string { return b.addresses[name] }

// Channels are the records of the channels bound, each as it stands, in the order of their names.
func (b *Bound) Channels() []*interfacev1.ChannelRecord {
	var out []*interfacev1.ChannelRecord
	for _, s := range b.surfaces {
		out = append(out, s.channelRecord())
	}
	return out
}

// Close unbinds every channel.
func (b *Bound) Close() error {
	for _, stop := range b.closers {
		stop()
	}
	b.closers = nil
	return nil
}
