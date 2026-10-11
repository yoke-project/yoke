package pkgfile

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/version"
)

// payload is a Zip archive whose root entries are a bundle's.
func payload(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range map[string]string{
		"yoke.yaml":                  "model: 1\nid: com.yoke.station\n",
		"manifests/clock.yaml":       "id: com.yoke.clock\n",
		"components/clock/bin/clock": "\x7fELF",
	} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, body)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func options() Options {
	return Options{
		Model:   1,
		ID:      "com.yoke.station",
		Version: "2.1.0",
		Arch:    "aarch64",
		Core:    ">=2.0 <3.0",
		BuiltAt: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC),
	}
}

func produce(t *testing.T, body []byte, o Options) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := Write(&b, body, o); err != nil {
		t.Fatalf("producing: %v", err)
	}
	return b.Bytes()
}

func footer(t *testing.T, file []byte) *Footer {
	t.Helper()
	f, err := ReadFooter(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatalf("reading the footer: %v", err)
	}
	return f
}

// keys is a trust store holding the given public keys.
type keys []crypto.PublicKey

func (k keys) Key(id [32]byte) (crypto.PublicKey, bool) {
	for _, pub := range k {
		if KeyID(pub) == id {
			return pub, true
		}
	}
	return nil, false
}

func host() Host {
	return Host{Arch: "aarch64", Models: []uint16{1}, Core: version.MustParse("2.4.0")}
}

func kind(err error) Kind {
	var r *Refusal
	if errors.As(err, &r) {
		return r.Kind
	}
	return -1
}

func refusal(t *testing.T, err error, want Kind, mentions ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("passed; want a refusal of kind %v", want)
	}
	if got := kind(err); got != want {
		t.Fatalf("refused as %v, want %v: %v", got, want, err)
	}
	for _, m := range mentions {
		if !strings.Contains(err.Error(), m) {
			t.Errorf("the refusal does not name %q: %v", m, err)
		}
	}
}

// remeta rewrites META in a produced file through edit and recomputes its CRC.
func remeta(file []byte, edit func(meta []byte)) []byte {
	out := bytes.Clone(file)
	meta := out[len(out)-256:]
	edit(meta)
	binary.LittleEndian.PutUint32(meta[252:], crc32.ChecksumIEEE(meta[:252]))
	return out
}

// std: yoke:the-package.01
func TestTheFooterIsLaidOutAsTheFormatSays(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	o := options()
	o.Sign = priv
	body := payload(t)
	file := produce(t, body, o)
	if len(file) != len(body)+512 {
		t.Fatalf("the file is %d bytes; the payload is %d and the footer must be 512", len(file), len(body))
	}
	meta := file[len(file)-256:]
	le := binary.LittleEndian
	if string(meta[0:8]) != "YOKEPKG1" || le.Uint16(meta[8:]) != 1 || le.Uint16(meta[10:]) != 252 {
		t.Fatalf("magic, format, covered length: %q %d %d", meta[0:8], le.Uint16(meta[8:]), le.Uint16(meta[10:]))
	}
	if le.Uint16(meta[12:]) != 1 || meta[14] != 1 || meta[15] != 1 || meta[16] != 0 {
		t.Errorf("model, hash, signature, seal: %d %d %d %d", le.Uint16(meta[12:]), meta[14], meta[15], meta[16])
	}
	if le.Uint64(meta[24:]) != uint64(len(body)) {
		t.Errorf("payload_size %d, want %d", le.Uint64(meta[24:]), len(body))
	}
	if le.Uint64(meta[32:]) != uint64(o.BuiltAt.UnixNano()) {
		t.Errorf("built_at %d", le.Uint64(meta[32:]))
	}
	if d := sha256.Sum256(body); !bytes.Equal(meta[40:72], d[:]) {
		t.Errorf("digest is not the payload's SHA-256")
	}
	pub := priv.Public()
	der, _ := x509.MarshalPKIXPublicKey(pub)
	if d := sha256.Sum256(der); !bytes.Equal(meta[72:104], d[:]) {
		t.Errorf("key_id is not the SHA-256 of the signing public key")
	}
	for _, f := range []struct {
		at, width int
		want      string
	}{{104, 48, o.ID}, {152, 32, o.Version}, {184, 12, o.Arch}, {196, 24, o.Core}} {
		field := meta[f.at : f.at+f.width]
		if !bytes.HasPrefix(field, []byte(f.want)) || !bytes.Equal(field[len(f.want):], make([]byte, f.width-len(f.want))) {
			t.Errorf("field at %d holds %q, want %q zero-padded", f.at, field, f.want)
		}
	}
	for _, r := range [][2]int{{17, 24}, {220, 252}} {
		if !bytes.Equal(meta[r[0]:r[1]], make([]byte, r[1]-r[0])) {
			t.Errorf("reserved bytes %d–%d are not zero", r[0], r[1])
		}
	}
	if le.Uint32(meta[252:]) != crc32.ChecksumIEEE(meta[:252]) {
		t.Errorf("the CRC does not cover META[0, 252)")
	}
	sig := file[len(file)-512 : len(file)-256]
	if !ed25519.Verify(pub.(ed25519.PublicKey), meta, sig[:64]) || !bytes.Equal(sig[64:], make([]byte, 192)) {
		t.Errorf("SIG is not the signature over META, left-aligned and zero-padded")
	}
}

// std: yoke:the-package.02
func TestAPackageInClearOpensWithAnOrdinaryZipReader(t *testing.T) {
	file := produce(t, payload(t), options())
	z, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatalf("the Zip reader does not open the package: %v", err)
	}
	names := map[string]bool{}
	for _, e := range z.File {
		r, err := e.Open()
		if err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		if _, err := io.ReadAll(r); err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		names[e.Name] = true
	}
	for _, want := range []string{"yoke.yaml", "manifests/clock.yaml", "components/clock/bin/clock"} {
		if !names[want] {
			t.Errorf("%s is not listed", want)
		}
	}
}

// guarded fails every read that reaches before the footer.
type guarded struct {
	file []byte
	read *bool
}

func (g guarded) ReadAt(p []byte, off int64) (int, error) {
	if off < int64(len(g.file)-512) {
		*g.read = true
		return 0, errors.New("a byte before the footer was read")
	}
	return bytes.NewReader(g.file).ReadAt(p, off)
}

// std: yoke:the-package.03
func TestNotAPackageAndDamagedAreReportedApartFromTrust(t *testing.T) {
	file := produce(t, payload(t), options())
	short := file[len(file)-300:]
	if _, err := ReadFooter(bytes.NewReader(short), int64(len(short))); kind(err) != NotAPackage {
		t.Errorf("a file of 300 bytes: %v", err)
	}
	notMagic := remeta(file, func(m []byte) { copy(m, "ZIPPKG01") })
	wrongFormat := remeta(file, func(m []byte) { binary.LittleEndian.PutUint16(m[8:], 2) })
	damaged := bytes.Clone(file)
	damaged[len(damaged)-200] ^= 0xff
	for _, c := range []struct {
		name string
		file []byte
		want Kind
	}{{"a foreign magic", notMagic, NotAPackage}, {"format 2", wrongFormat, NotAPackage}, {"a changed byte", damaged, Damaged}} {
		read := false
		_, err := ReadFooter(guarded{c.file, &read}, int64(len(c.file)))
		if kind(err) != c.want {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
		if read {
			t.Errorf("%s: a byte past the footer was read", c.name)
		}
	}
}

// std: yoke:the-package.04
func TestReservedBytesThatAreNotZeroAreRefused(t *testing.T) {
	for _, at := range []int{17, 230} {
		file := remeta(produce(t, payload(t), options()), func(m []byte) { m[at] = 1 })
		_, err := ReadFooter(bytes.NewReader(file), int64(len(file)))
		refusal(t, err, Unreadable, "reserved")
	}
}

// std: yoke:the-package.05
func TestAValueThatDoesNotFitIsRefusedNeverTruncated(t *testing.T) {
	for _, c := range []struct {
		field string
		width int
		set   func(*Options, string)
	}{
		{"id", 48, func(o *Options, s string) { o.ID = s }},
		{"version", 32, func(o *Options, s string) { o.Version = s }},
		{"arch", 12, func(o *Options, s string) { o.Arch = s }},
		{"core", 24, func(o *Options, s string) { o.Core = s }},
	} {
		o := options()
		c.set(&o, strings.Repeat("a", c.width+1))
		var b bytes.Buffer
		err := Write(&b, payload(t), o)
		refusal(t, err, Unproducible, c.field, strconv.Itoa(c.width))
		if b.Len() != 0 {
			t.Errorf("%s: %d bytes were written", c.field, b.Len())
		}
	}
}

// std: yoke:the-package.06
func TestAnUnknownAlgorithmCannotBeDecidedAndSaysWhich(t *testing.T) {
	file := produce(t, payload(t), options())
	for _, c := range []struct {
		field string
		at    int
	}{{"hash_id", 14}, {"sig_id", 15}, {"seal_id", 16}} {
		f := footer(t, remeta(file, func(m []byte) { m[c.at] = 9 }))
		err := f.Check(keys{}, Policy{}, host())
		refusal(t, err, Undecidable, c.field, "9")
	}
}

// std: yoke:the-package.07
func TestASignatureIsVerifiedAgainstATrustedKey(t *testing.T) {
	edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		key  crypto.Signer
		pub  crypto.PublicKey
		id   byte
	}{{"Ed25519", edPriv, edPub, 1}, {"RSA-2048", rsaPriv, &rsaPriv.PublicKey, 2}} {
		o := options()
		o.Sign = c.key
		f := footer(t, produce(t, payload(t), o))
		if f.Meta.SigID != c.id {
			t.Errorf("%s: sig_id %d, want %d", c.name, f.Meta.SigID, c.id)
		}
		der, _ := x509.MarshalPKIXPublicKey(c.pub)
		if f.Meta.KeyID != sha256.Sum256(der) {
			t.Errorf("%s: key_id is not the SHA-256 of the public key", c.name)
		}
		if err := f.Check(keys{c.pub}, Policy{RequireSignature: true}, host()); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// std: yoke:the-package.08
func TestAlteredMetadataFailsTheSignatureWithItsCRCRecomputed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	o := options()
	o.Sign = priv
	file := remeta(produce(t, payload(t), o), func(m []byte) { m[40] ^= 0xff })
	f := footer(t, file) // the CRC passes
	refusal(t, f.Check(keys{pub}, Policy{}, host()), Trust)
}

// std: yoke:the-package.09
func TestAnUntrustedKeyIsAFailureOfTrustOnTheFooterAlone(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	o := options()
	o.Sign = priv
	file := produce(t, payload(t), o)
	read := false
	f, err := ReadFooter(guarded{file, &read}, int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	refusal(t, f.Check(keys{other}, Policy{}, host()), Trust, "key")
	if read {
		t.Error("a byte of the payload was read")
	}
}

// std: yoke:the-package.10
func TestAnUnsignedPackageAndThePolicy(t *testing.T) {
	f := footer(t, produce(t, payload(t), options()))
	refusal(t, f.Check(keys{}, Policy{RequireSignature: true}, host()), Trust, "signature")
	if err := f.Check(keys{}, Policy{}, host()); err != nil {
		t.Errorf("an unsigned package under a policy that does not require one: %v", err)
	}
}

// std: yoke:the-package.11
func TestArchitectureModelAndCoreAreComparedAgainstTheHost(t *testing.T) {
	for _, c := range []struct {
		edit  func(*Options)
		names string
	}{
		{func(o *Options) { o.Arch = "x86_64" }, "x86_64"},
		{func(o *Options) { o.Model = 2 }, "model"},
		{func(o *Options) { o.Core = ">=3.0" }, ">=3.0"},
		{func(o *Options) { o.Arch = "any" }, "any"},
	} {
		o := options()
		c.edit(&o)
		f := footer(t, produce(t, payload(t), o))
		refusal(t, f.Check(keys{}, Policy{}, host()), Unwanted, c.names)
	}
	if err := footer(t, produce(t, payload(t), options())).Check(keys{}, Policy{}, host()); err != nil {
		t.Errorf("a matching package: %v", err)
	}
}

// std: yoke:the-package.12
func TestAVersionThatIsNotSemanticIsRefusedAPrereleaseIsNot(t *testing.T) {
	o := options()
	o.Version = "2.1"
	refusal(t, footer(t, produce(t, payload(t), o)).Check(keys{}, Policy{}, host()), Invalid, "2.1")
	for _, v := range []string{"2.1.0-rc.1", "2.1.0"} {
		o.Version = v
		if err := footer(t, produce(t, payload(t), o)).Check(keys{}, Policy{}, host()); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
}

// std: yoke:the-package.13
func TestASealedPackageOpensWithItsCredential(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	credential := bytes.Repeat([]byte{7}, 32)
	o := options()
	o.Sign = priv
	o.Seal = credential
	body := payload(t)
	file := produce(t, body, o)
	f := footer(t, file)
	if f.Meta.SealID != 1 {
		t.Fatalf("seal_id %d", f.Meta.SealID)
	}
	if err := f.Check(keys{pub}, Policy{RequireSignature: true}, host()); err != nil {
		t.Fatalf("the footer, checked with no credential: %v", err)
	}
	got, err := f.Open(bytes.NewReader(file), credential)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("opened with its credential: %v", err)
	}
	refusal(t, ignore(f.Open(bytes.NewReader(file), bytes.Repeat([]byte{8}, 32))), Seal)
	refusal(t, ignore(f.Open(bytes.NewReader(file), nil)), Seal, "credential")
	tampered := bytes.Clone(file)
	tampered[len(tampered)-513] ^= 0xff // the last byte of the tag
	refusal(t, ignore(f.Open(bytes.NewReader(tampered), credential)), Seal)
}

func ignore(_ []byte, err error) error { return err }

// std: yoke:the-package.14
func TestTheDigestCoversThePayloadAfterUnsealing(t *testing.T) {
	body := payload(t)
	clear := footer(t, produce(t, body, options()))
	o := options()
	o.Seal = bytes.Repeat([]byte{7}, 32)
	sealed := footer(t, produce(t, body, o))
	if clear.Meta.Digest != sealed.Meta.Digest {
		t.Error("sealing changed the digest")
	}
	file := produce(t, body, options())
	file[10] ^= 0xff
	f := footer(t, file)
	refusal(t, ignore(f.Open(bytes.NewReader(file), nil)), Digest)
}

// std: yoke:the-package.15
func TestNothingInTheVerificationReadsTheClock(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, 1).UTC(), time.Date(2126, 1, 1, 0, 0, 0, 0, time.UTC)} {
		o := options()
		o.BuiltAt = at
		file := produce(t, payload(t), o)
		f := footer(t, file)
		if err := f.Check(keys{}, Policy{}, host()); err != nil {
			t.Errorf("built %v: %v", at, err)
		}
		if _, err := f.Open(bytes.NewReader(file), nil); err != nil {
			t.Errorf("built %v: %v", at, err)
		}
		if !f.Meta.BuiltAt.Equal(at) {
			t.Errorf("built_at read back as %v, written %v", f.Meta.BuiltAt, at)
		}
	}
}

// std: yoke:the-package.16
func TestAPackageIsNamedByItsIdentityAndVersion(t *testing.T) {
	if got := Name("com.yoke.station", "2.1.0"); got != "com.yoke.station-2.1.0.yoke" {
		t.Errorf("named %q", got)
	}
}
