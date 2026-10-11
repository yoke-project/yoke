package truststore

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/pkgfile"
	"github.com/yoke-project/yoke/internal/version"
)

// The tests run as an ordinary account, which stands for root here.
func init() { root = uint32(os.Getuid()) }

func public(t *testing.T, pub crypto.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func row(t *testing.T, label, algorithm string, pub crypto.PublicKey) string {
	id := pkgfile.KeyID(pub)
	return "  - id: " + hex.EncodeToString(id[:]) + "\n    label: " + label + "\n    algorithm: " + algorithm +
		"\n    public: " + public(t, pub) + "\n"
}

func file(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "packages.conf")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func none(string) string { return "" }

func refused(t *testing.T, err error, mentions ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("read; want a refusal")
	}
	for _, m := range mentions {
		if !strings.Contains(err.Error(), m) {
			t.Errorf("the refusal does not name %q: %v", m, err)
		}
	}
}

var edPub, edPriv, _ = ed25519.GenerateKey(rand.Reader)

func rsaKey(t *testing.T, bits int) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// std: yoke:the-trust-store.01
func TestTheFileIsRead(t *testing.T) {
	r := rsaKey(t, 2048)
	seal := file(t, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))+"\n", 0o600)
	p := file(t, "require_signature: true\nkeys:\n"+row(t, "vendor-2026", "ed25519", edPub)+
		row(t, "vendor-2024", "rsa-2048", &r.PublicKey)+"seal:\n  credential: "+seal+"\n", 0o644)
	s, err := Load(p, none)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Policy().RequireSignature {
		t.Error("the policy does not require a signature")
	}
	for label, pub := range map[string]crypto.PublicKey{"vendor-2026": edPub, "vendor-2024": &r.PublicKey} {
		got, ok := s.Key(pkgfile.KeyID(pub))
		if !ok || pkgfile.KeyID(got) != pkgfile.KeyID(pub) {
			t.Errorf("%s is not trusted under its id", label)
		}
		if s.Label(pkgfile.KeyID(pub)) != label {
			t.Errorf("the label of %s is %q", label, s.Label(pkgfile.KeyID(pub)))
		}
	}
	if s.SealPath() != seal {
		t.Errorf("the credential is at %q, want %q", s.SealPath(), seal)
	}
	bare, err := Load(file(t, "keys: []\n", 0o644), none)
	if err != nil {
		t.Fatal(err)
	}
	if bare.Policy().RequireSignature || bare.SealPath() != "" {
		t.Errorf("a file saying nothing: require %v, credential at %q", bare.Policy().RequireSignature, bare.SealPath())
	}
}

// std: yoke:the-trust-store.02
func TestAFileNotRootsOrWritableByOthersIsRefused(t *testing.T) {
	body := "keys:\n" + row(t, "vendor", "ed25519", edPub)
	p := file(t, body, 0o644)
	root = uint32(os.Getuid()) + 1
	_, err := Load(p, none)
	root = uint32(os.Getuid())
	refused(t, err, "owned by")
	for _, mode := range []os.FileMode{0o664, 0o646} {
		_, err := Load(file(t, body, mode), none)
		refused(t, err, "writable", fmt.Sprintf("%#o", mode))
	}
}

// std: yoke:the-trust-store.03
func TestAKeyWhoseIDIsNotItsOwnDigestIsRefused(t *testing.T) {
	r := rsaKey(t, 2048)
	first := pkgfile.KeyID(edPub)
	wrong := row(t, "second", "rsa-2048", &r.PublicKey)
	id := pkgfile.KeyID(&r.PublicKey)
	wrong = strings.Replace(wrong, hex.EncodeToString(id[:]), hex.EncodeToString(first[:]), 1)
	_, err := Load(file(t, "keys:\n"+row(t, "first", "ed25519", edPub)+wrong, 0o644), none)
	refused(t, err, "keys[1]")
}

// std: yoke:the-trust-store.04
func TestAnUnknownKeyInThePolicyFileIsRefused(t *testing.T) {
	for _, c := range []struct{ body, names string }{
		{"keys: []\nallow_unsigned: true\n", "allow_unsigned"},
		{"keys:\n" + row(t, "vendor", "ed25519", edPub) + "    expires: 2030-01-01\n", "expires"},
		{"keys: []\nseal:\n  credential: /etc/yoke/seal.key\n  cipher: aes\n", "cipher"},
	} {
		_, err := Load(file(t, c.body, 0o644), none)
		refused(t, err, c.names)
	}
}

// std: yoke:the-trust-store.05
func TestAnAlgorithmIsEd25519OrRSA2048AndThePublicKeyMustBeOne(t *testing.T) {
	big := rsaKey(t, 4096)
	r := rsaKey(t, 2048)
	for _, c := range []struct{ body, names string }{
		{"keys:\n" + row(t, "vendor", "dsa", edPub), "dsa"},
		{"keys:\n" + row(t, "vendor", "ed25519", &r.PublicKey), "ed25519"},
		{"keys:\n" + row(t, "vendor", "rsa-2048", &big.PublicKey), "4096"},
	} {
		_, err := Load(file(t, c.body, 0o644), none)
		refused(t, err, "keys[0]", c.names)
	}
}

func signed(t *testing.T, key crypto.Signer) *pkgfile.Footer {
	t.Helper()
	var b bytes.Buffer
	err := pkgfile.Write(&b, []byte("payload"), pkgfile.Options{Model: 1, ID: "com.yoke.station", Version: "2.1.0",
		Arch: "aarch64", Core: ">=2.0", BuiltAt: time.Unix(0, 0), Sign: key})
	if err != nil {
		t.Fatal(err)
	}
	f, err := pkgfile.ReadFooter(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// std: yoke:the-trust-store.06
func TestTheSetIsWhatIsTrusted(t *testing.T) {
	newPub, newPriv, _ := ed25519.GenerateKey(rand.Reader)
	old, current := signed(t, edPriv), signed(t, newPriv)
	host := pkgfile.Host{Arch: "aarch64", Models: []uint16{1}, Core: version.MustParse("2.4.0")}
	both, err := Load(file(t, "require_signature: true\nkeys:\n"+row(t, "old", "ed25519", edPub)+row(t, "new", "ed25519", newPub), 0o644), none)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []*pkgfile.Footer{old, current} {
		if err := f.Check(both, both.Policy(), host); err != nil {
			t.Errorf("with both keys: %v", err)
		}
	}
	after, err := Load(file(t, "require_signature: true\nkeys:\n"+row(t, "new", "ed25519", newPub), 0o644), none)
	if err != nil {
		t.Fatal(err)
	}
	var r *pkgfile.Refusal
	if err := old.Check(after, after.Policy(), host); !errors.As(err, &r) || r.Kind != pkgfile.Trust {
		t.Errorf("signed by the removed key: %v", err)
	}
	if err := current.Check(after, after.Policy(), host); err != nil {
		t.Errorf("signed by the key kept: %v", err)
	}
}

// std: yoke:the-trust-store.07
func TestAVariableMayTightenThePolicyAndNeverLoosenIt(t *testing.T) {
	loose := file(t, "keys: []\n", 0o644)
	strict := file(t, "require_signature: true\nkeys: []\n", 0o644)
	for _, value := range []string{"1", "0", "false", ""} {
		env := func(k string) string {
			if k == "YOKE_REQUIRE_SIGNATURE" {
				return value
			}
			return ""
		}
		l, err := Load(loose, env)
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Policy().RequireSignature; got != (value == "1") {
			t.Errorf("a loose file under %q: require %v", value, got)
		}
		s, err := Load(strict, env)
		if err != nil {
			t.Fatal(err)
		}
		if !s.Policy().RequireSignature {
			t.Errorf("a strict file under %q stopped requiring a signature", value)
		}
	}
}

// std: yoke:the-trust-store.08
func TestACredentialAtRestIsReadFromItsPath(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	store := func(credential string, mode os.FileMode) *Store {
		seal := file(t, credential, mode)
		s, err := Load(file(t, "keys: []\nseal:\n  credential: "+seal+"\n", 0o644), none)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := store(base64.StdEncoding.EncodeToString(key)+"\n", 0o600)
	got, err := s.Credential("")
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("the credential at rest: %v", err)
	}
	if w := s.Weaker(); len(w) != 1 || w[0] != "seal.credential.at_rest" {
		t.Errorf("weaker arrangements: %v", w)
	}
	_, err = store(base64.StdEncoding.EncodeToString(key), 0o640).Credential("")
	refused(t, err, "0640")
	_, err = store(base64.StdEncoding.EncodeToString(key[:16]), 0o600).Credential("")
	refused(t, err, "16")
}

// std: yoke:the-trust-store.09
func TestWithNoPathTheCredentialIsTheOneSupplied(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	s, err := Load(file(t, "keys: []\n", 0o644), none)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Credential(base64.StdEncoding.EncodeToString(key))
	if err != nil || !bytes.Equal(got, key) {
		t.Errorf("supplied: %v", err)
	}
	got, err = s.Credential("")
	if err != nil || got != nil {
		t.Errorf("none supplied: %v, %v", got, err)
	}
	if len(s.Weaker()) != 0 {
		t.Errorf("weaker arrangements: %v", s.Weaker())
	}
}
