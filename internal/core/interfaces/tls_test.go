package interfaces_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/encoding/protojson"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/gate"
)

// material writes a self-signed certificate and its key for each channel into dir, as whoever provisions
// a device would, and answers the certificates by channel.
func material(t *testing.T, dir string, channels ...string) map[string]*x509.Certificate {
	t.Helper()
	os.MkdirAll(dir, 0o700)
	out := map[string]*x509.Certificate{}
	for _, ch := range channels {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: ch}, NotBefore: time.Now().Add(-time.Hour),
			NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		k, _ := x509.MarshalPKCS8PrivateKey(key)
		os.WriteFile(filepath.Join(dir, ch+".crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
		os.WriteFile(filepath.Join(dir, ch+".key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: k}), 0o600)
		out[ch], _ = x509.ParseCertificate(der)
	}
	return out
}

func routable(name, transport string, t *testing.T) gate.Channel {
	return gate.Channel{Name: name, Transport: transport, Clients: "multiple",
		Address: &gate.Address{Class: "routable", Host: "127.0.0.1", Port: freePort(t), Transport: "encrypted", Caller: "authenticated"}}
}

// std: yoke:channels-bound.06
func TestARoutableChannelSpeaksTLS13AloneWithItsProjectionsALPN(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	certs := material(t, dir, "panel", "service")
	b, err := interfaces.BindServing(root(t), dir, mode, []gate.Channel{routable("panel", "local", t), routable("service", "http+ws", t)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	material(t, dir, "panel", "service") // replaced on disk after binding
	for name, alpn := range map[string]string{"panel": "h2", "service": "http/1.1"} {
		old := &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12}
		if c, err := tls.Dial("tcp", b.Address(name), old); err == nil {
			c.Close()
			t.Errorf("%s accepted TLS 1.2", name)
		}
		asked := false
		c, err := tls.Dial("tcp", b.Address(name), &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13, NextProtos: []string{"h2", "http/1.1"},
			GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
				asked = true
				return &tls.Certificate{}, nil
			}})
		if err != nil {
			t.Fatalf("%s over TLS 1.3: %v", name, err)
		}
		st := c.ConnectionState()
		c.Close()
		if st.Version != tls.VersionTLS13 || st.NegotiatedProtocol != alpn {
			t.Errorf("%s negotiated %x and %q", name, st.Version, st.NegotiatedProtocol)
		}
		if len(st.PeerCertificates) == 0 || !st.PeerCertificates[0].Equal(certs[name]) {
			t.Errorf("%s did not present the certificate it was bound with", name)
		}
		if asked {
			t.Errorf("%s asked for a client certificate", name)
		}
	}
}

// std: yoke:channels-bound.07
func TestOnARoutableChannelAClientWithNoCallerIsRefused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	material(t, dir, "panel", "service")
	var published []string
	b, err := interfaces.BindServing(root(t), dir, mode, []gate.Channel{routable("panel", "local", t), routable("service", "http+ws", t)},
		func(ch gate.Channel) interfacev1.InterfaceServer {
			return interfaces.NewSurface(interfaces.Config{Channel: ch, Publish: func(e event.Event) { published = append(published, e.Type) }})
		})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	trust := &tls.Config{InsecureSkipVerify: true}
	conn, err := grpc.NewClient(b.Address("panel"), grpc.WithTransportCredentials(credentials.NewTLS(trust)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err == nil {
		err = stream.Send(&interfacev1.ClientFrame{Call: "r", Carries: &interfacev1.ClientFrame_Request{Request: &interfacev1.Request{Version: 1,
			Operation: &interfacev1.Request_Read{Read: &interfacev1.Read{Kind: "instance"}}}}})
	}
	if err == nil {
		_, err = stream.Recv()
	}
	if refusalIn(err).GetCode() != "auth.required" {
		t.Errorf("the local projection answered %v", err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: trust}}
	resp, err := client.Get("https://" + b.Address("service") + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	var ref interfacev1.Refusal
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	if err := protojson.Unmarshal(body[:n], &ref); err != nil || ref.GetCode() != "auth.required" {
		t.Errorf("the browser projection answered %d %s", resp.StatusCode, body[:n])
	}
	if len(published) != 0 {
		t.Errorf("published %v", published)
	}
}
