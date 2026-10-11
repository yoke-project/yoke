package compose

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/pkgfile"
)

func run(args ...string) (int, string) {
	var out bytes.Buffer
	code := Run(args, &out, &out)
	return code, out.String()
}

// std: yoke:the-composer.09
func TestTheCommandLine(t *testing.T) {
	d, o := t.TempDir(), t.TempDir()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	key := filepath.Join(t.TempDir(), "key.pem")
	os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
	credential := filepath.Join(t.TempDir(), "seal.key")
	os.WriteFile(credential, []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32))+"\n"), 0o600)

	if code, out := run("init", "com.yoke.station", "--dir", d, "--arch", "aarch64"); code != 0 {
		t.Fatalf("init: %d %s", code, out)
	}
	if code, out := run("check", "--dir", d); code == 0 || !strings.Contains(out, "field.required") {
		t.Errorf("check with no unit: %d %s", code, out)
	}
	body := "#!/bin/sh\n"
	os.WriteFile(filepath.Join(d, "components", "a"), []byte(body), 0o755)
	f, _ := os.OpenFile(filepath.Join(d, "yoke.yaml"), os.O_APPEND|os.O_WRONLY, 0)
	fmt.Fprintf(f, "units:\n  a: { kind: oneshot, exec: components/a, digest: \"sha256:%x\" }\n", sha256.Sum256([]byte(body)))
	f.Close()
	code, out := run("build", "--dir", d, "--out", o, "--sign", key, "--seal", credential)
	if code != 0 {
		t.Fatalf("build: %d %s", code, out)
	}
	path := strings.TrimSpace(out[strings.LastIndex(strings.TrimSpace(out), "\n")+1:])
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the path printed, %q: %v", path, err)
	}
	footer, err := pkgfile.ReadFooter(bytes.NewReader(b), int64(len(b)))
	if err != nil || footer.Meta.SigID != pkgfile.SigEd25519 || footer.Meta.SealID != pkgfile.SealAES256GCM {
		t.Errorf("the package: %v %+v", err, footer)
	}
	if code, out := run("publish"); code == 0 || !strings.Contains(out, "publish") || !strings.Contains(out, "build") {
		t.Errorf("an unknown verb: %d %s", code, out)
	}
}
