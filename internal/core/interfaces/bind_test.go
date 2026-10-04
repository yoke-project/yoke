package interfaces_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/gate"
)

const mode = 0o660

func root(t *testing.T) string {
	t.Helper()
	dir, _ := os.MkdirTemp("", "yi")
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
}

// std: yoke:channels-bound.01
func TestALocalChannelIsASocketAndALoopbackChannelAPort(t *testing.T) {
	r := root(t)
	port := freePort(t)
	bound, err := interfaces.Bind(r, mode, []gate.Channel{
		{Name: "panel", Transport: "local", Clients: "single"},
		{Name: "remote", Transport: "http+ws", Clients: "multiple", Address: &gate.Address{Class: "loopback", Port: port}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	socket := filepath.Join(r, "interfaces", "panel.sock")
	if info, err := os.Stat(socket); err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != mode {
		t.Errorf("the local channel is %v %v, want a socket of mode %o at %s", info, err, mode, socket)
	}
	if c, err := net.Dial("tcp", "127.0.0.1:"+port); err != nil {
		t.Errorf("the loopback channel does not listen: %v", err)
	} else {
		c.Close()
	}
	if got := bound.Address("panel"); got != socket {
		t.Errorf("the local channel's address is %q", got)
	}
	if got := bound.Address("remote"); got != "127.0.0.1:"+port {
		t.Errorf("the loopback channel's address is %q", got)
	}
}

// std: yoke:channels-bound.02
func TestBindingIsFatalAndARoutableChannelIsRefused(t *testing.T) {
	r := root(t)
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	taken := fmt.Sprint(held.Addr().(*net.TCPAddr).Port)
	_, err = interfaces.Bind(r, mode, []gate.Channel{
		{Name: "panel", Transport: "local", Clients: "single"},
		{Name: "remote", Transport: "http+ws", Clients: "multiple", Address: &gate.Address{Class: "loopback", Port: taken}},
	})
	if err == nil || !strings.Contains(err.Error(), "remote") {
		t.Errorf("binding on a port in use gave %v", err)
	}
	if _, err := os.Stat(filepath.Join(r, "interfaces", "panel.sock")); err == nil {
		t.Error("the local channel bound before the failure is still there")
	}
	_, err = interfaces.Bind(r, mode, []gate.Channel{
		{Name: "panel", Transport: "local", Clients: "single"},
		{Name: "service", Transport: "http+ws", Clients: "multiple", Address: &gate.Address{Class: "routable", Host: "0.0.0.0", Port: freePort(t), Transport: "tls", Caller: "credential"}},
	})
	if err == nil || !strings.Contains(err.Error(), "service") || !strings.Contains(err.Error(), "routable") {
		t.Errorf("binding a routable channel gave %v", err)
	}
	if _, err := os.Stat(filepath.Join(r, "interfaces", "panel.sock")); err == nil {
		t.Error("the local channel bound beside the routable one is still there")
	}
}

// std: yoke:channels-bound.03
func TestEachChannelAnswersInItsProjection(t *testing.T) {
	r := root(t)
	port := freePort(t)
	bound, err := interfaces.Bind(r, mode, []gate.Channel{
		{Name: "panel", Transport: "local", Clients: "single"},
		{Name: "remote", Transport: "http+ws", Clients: "multiple", Address: &gate.Address{Class: "loopback", Port: port}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	conn, err := grpc.NewClient("unix://"+bound.Address("panel"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err == nil {
		_, err = stream.Recv()
	}
	if st, _ := status.FromError(err); st.Code() == codes.Unavailable || strings.Contains(st.Message(), "unknown service") {
		t.Errorf("the local channel does not answer as the interface service: %v", err)
	}
	resp, err := http.Get("http://" + bound.Address("remote") + "/index.html")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("a path outside /v1 answered %d", resp.StatusCode)
	}
}
