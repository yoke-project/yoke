package interfaces_test

import (
	"context"
	"crypto/x509"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke-sdk-go/base"
	"github.com/yoke-project/yoke-sdk-go/iface"
)

// std: yoke:a-go-client-across-a-routable-address.01
func TestThroughTheCoreTheGoLibrarySignsInAndAttachesAgainWithItsToken(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	certs := material(t, filepath.Join(dir, "state", "tls"), "panel")
	_, lines := startCoreIn(t, dir, "", `# The test binary, %s, runs nothing here; %s is unused.
units: {}
channels:
  panel: { transport: local, clients: multiple, address: { class: routable, host: 127.0.0.1, port: "`+port+`", security: { transport: encrypted, caller: authenticated } } }
`)
	var said []string
	deadline := time.After(20 * time.Second)
	for bound := false; !bound; {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			bound = strings.Contains(line, "msg=channel") && strings.Contains(line, "name=panel")
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
	roots := x509.NewCertPool()
	roots.AddCert(certs["panel"])
	address := "127.0.0.1:" + port
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	first, err := iface.Attach(ctx, address, iface.TLS(roots, ""), iface.Password("alice", "hunter2x"))
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}
	signed := first.Established
	if first.Picture == nil || signed.GetAccount() != "alice" || signed.GetToken() == "" || signed.GetExpires() == nil {
		t.Errorf("signing in surfaced the picture %v and established %v", first.Picture, signed)
	}
	first.Close()

	again, err := iface.Attach(ctx, address, iface.TLS(roots, ""), iface.Token(signed.GetToken()))
	if err != nil {
		t.Fatalf("attaching with the token: %v", err)
	}
	if again.Picture == nil || again.Established.GetAccount() != "alice" {
		t.Errorf("attaching with the token surfaced the picture %v and established %v", again.Picture, again.Established)
	}
	again.Close()

	if a, err := iface.Attach(ctx, address, iface.TLS(roots, ""), iface.Password("alice", "wrong")); !base.IsRefusal(err, "auth.invalid") {
		if a != nil {
			a.Close()
		}
		t.Errorf("a secret the host refuses was answered %v", err)
	}
}
