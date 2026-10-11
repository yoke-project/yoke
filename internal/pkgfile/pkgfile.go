// Package pkgfile is the package: the transport form of an application, a Zip payload with a 512-byte
// footer appended — `SIG` then `META` — that a host decides on before it touches the payload.
//
// Reading follows the order that trusts nothing before it is checked: ReadFooter reads the last 512 bytes
// and checks the magic, the format and the CRC; Check verifies the signature against the trusted keys and
// compares the footer against the host; Open unseals, where the payload is sealed, and recomputes the
// digest. Each failure is a Refusal of its own kind, and none of the three checks does another's work.
package pkgfile

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/yoke-project/yoke/internal/version"
)

// The figures of format 1.
const (
	FooterSize = 512
	blockSize  = 256
	covered    = 252
	magic      = "YOKEPKG1"
	format     = 1
	nonceSize  = 12
	tagSize    = 16
	sealKey    = 32
)

// The algorithm identifiers.
const (
	HashSHA256 byte = 1

	SigNone    byte = 0
	SigEd25519 byte = 1
	SigRSA2048 byte = 2

	SealNone      byte = 0
	SealAES256GCM byte = 1
)

// The text fields: where each sits in META and how wide it is.
var fields = []struct {
	name      string
	at, width int
}{{"id", 104, 48}, {"version", 152, 32}, {"arch", 184, 12}, {"core", 196, 24}}

// The reserved ranges of META, which format 1 requires to be zero.
var reserved = [][2]int{{17, 24}, {220, 252}}

// Kind is what a refusal is about. Each step of the order names its own.
type Kind int

const (
	// NotAPackage — the file is not a package of a format this reader knows.
	NotAPackage Kind = iota
	// Damaged — the footer or the payload region is not what its own figures say.
	Damaged
	// Unreadable — a footer of format 1 that format 1 does not allow.
	Unreadable
	// Undecidable — an algorithm this host does not know: it cannot decide, which is not a decision against the file.
	Undecidable
	// Trust — the signature did not establish authenticity, or the policy required one that is absent.
	Trust
	// Unwanted — this host does not want this file: its architecture, its model or its Core range.
	Unwanted
	// Invalid — a value the footer carries cannot be what it claims, as a version that is not a semantic one.
	Invalid
	// Seal — the payload could not be unsealed: no credential, the wrong one, or a tag that does not verify.
	Seal
	// Digest — the payload is not the payload the metadata describes.
	Digest
	// Unproducible — a package cannot be produced from what was given.
	Unproducible
)

var kinds = [...]string{"not a package", "damaged", "unreadable", "undecidable", "a failure of trust",
	"not wanted by this host", "invalid", "not unsealed", "not the payload described", "not producible"}

func (k Kind) String() string {
	if k < 0 || int(k) >= len(kinds) {
		return fmt.Sprintf("kind %d", int(k))
	}
	return kinds[k]
}

// Refusal is a package refused at one step, naming what it refused on.
type Refusal struct {
	Kind    Kind
	Field   string
	Message string
}

func (r *Refusal) Error() string { return r.Kind.String() + ": " + r.Message }

func refuse(k Kind, field, format string, args ...any) *Refusal {
	return &Refusal{Kind: k, Field: field, Message: fmt.Sprintf(format, args...)}
}

// Meta is the metadata block, decoded.
type Meta struct {
	Model       uint16
	HashID      byte
	SigID       byte
	SealID      byte
	PayloadSize uint64
	BuiltAt     time.Time
	Digest      [32]byte
	KeyID       [32]byte
	ID          string
	Version     string
	Arch        string
	Core        string
}

// Name is a package's file name: `<id>-<version>.yoke`.
func Name(id, version string) string { return id + "-" + version + ".yoke" }

// KeyID is the SHA-256 of a public key in its PKIX encoding, which is what `key_id` holds.
func KeyID(pub crypto.PublicKey) [32]byte {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(der)
}

// Options is what producing a package is given besides its payload.
type Options struct {
	Model   uint16
	ID      string
	Version string
	Arch    string
	Core    string
	BuiltAt time.Time
	// Seal, where given, is the 32-byte credential the payload is sealed with.
	Seal []byte
	// Sign, where given, is the key the metadata is signed with: Ed25519, or RSA of 2048 bits.
	Sign crypto.Signer
}

// Write produces a package from a payload: the payload region, sealed where Seal is given, then the
// footer. A value the footer cannot carry is refused and nothing is written.
func Write(w io.Writer, payload []byte, o Options) error {
	m := Meta{Model: o.Model, HashID: HashSHA256, BuiltAt: o.BuiltAt, ID: o.ID, Version: o.Version,
		Arch: o.Arch, Core: o.Core, Digest: sha256.Sum256(payload)}
	for _, f := range fields {
		if v := m.text(f.name); len(v) > f.width {
			return refuse(Unproducible, f.name, "%s is %d bytes and the field holds %d", f.name, len(v), f.width)
		}
	}
	region := payload
	if o.Seal != nil {
		gcm, err := aead(o.Seal)
		if err != nil {
			return refuse(Unproducible, "seal", "%v", err)
		}
		nonce := make([]byte, nonceSize)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		region = gcm.Seal(nonce, nonce, payload, nil)
		m.SealID = SealAES256GCM
	}
	m.PayloadSize = uint64(len(region))
	var sig [blockSize]byte
	if o.Sign != nil {
		id, err := signatureID(o.Sign.Public())
		if err != nil {
			return refuse(Unproducible, "sig_id", "%v", err)
		}
		m.SigID = id
		m.KeyID = KeyID(o.Sign.Public())
	}
	meta := m.encode()
	if o.Sign != nil {
		s, err := sign(o.Sign, m.SigID, meta[:])
		if err != nil {
			return err
		}
		copy(sig[:], s)
	}
	for _, part := range [][]byte{region, sig[:], meta[:]} {
		if _, err := w.Write(part); err != nil {
			return err
		}
	}
	return nil
}

func (m Meta) text(name string) string {
	switch name {
	case "id":
		return m.ID
	case "version":
		return m.Version
	case "arch":
		return m.Arch
	default:
		return m.Core
	}
}

func (m *Meta) setText(name, v string) {
	switch name {
	case "id":
		m.ID = v
	case "version":
		m.Version = v
	case "arch":
		m.Arch = v
	default:
		m.Core = v
	}
}

func (m Meta) encode() [blockSize]byte {
	var b [blockSize]byte
	le := binary.LittleEndian
	copy(b[0:8], magic)
	le.PutUint16(b[8:], format)
	le.PutUint16(b[10:], covered)
	le.PutUint16(b[12:], m.Model)
	b[14], b[15], b[16] = m.HashID, m.SigID, m.SealID
	le.PutUint64(b[24:], m.PayloadSize)
	le.PutUint64(b[32:], uint64(m.BuiltAt.UnixNano()))
	copy(b[40:72], m.Digest[:])
	copy(b[72:104], m.KeyID[:])
	for _, f := range fields {
		copy(b[f.at:f.at+f.width], m.text(f.name))
	}
	le.PutUint32(b[covered:], crc32.ChecksumIEEE(b[:covered]))
	return b
}

func signatureID(pub crypto.PublicKey) (byte, error) {
	switch k := pub.(type) {
	case ed25519.PublicKey:
		return SigEd25519, nil
	case *rsa.PublicKey:
		if k.N.BitLen() == 2048 {
			return SigRSA2048, nil
		}
		return 0, fmt.Errorf("an RSA key of %d bits: the format names RSA-2048", k.N.BitLen())
	default:
		return 0, fmt.Errorf("a key of type %T: the format names Ed25519 and RSA-2048", pub)
	}
}

func sign(key crypto.Signer, id byte, meta []byte) ([]byte, error) {
	if id == SigEd25519 {
		return key.Sign(rand.Reader, meta, crypto.Hash(0))
	}
	d := sha256.Sum256(meta)
	return key.Sign(rand.Reader, d[:], crypto.SHA256)
}

func aead(credential []byte) (cipher.AEAD, error) {
	if len(credential) != sealKey {
		return nil, fmt.Errorf("the credential is %d bytes; AES-256-GCM takes %d", len(credential), sealKey)
	}
	block, err := aes.NewCipher(credential)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Footer is a footer read and found to be one: its metadata, and the two blocks as they were read.
type Footer struct {
	Meta Meta
	meta [blockSize]byte
	sig  [blockSize]byte
}

// ReadFooter reads the last 512 bytes of a file of the given size — and nothing else — and checks the
// magic, then the format, then the CRC: step 1 of the order.
func ReadFooter(r io.ReaderAt, size int64) (*Footer, error) {
	if size < FooterSize {
		return nil, refuse(NotAPackage, "", "the file is %d bytes, shorter than a footer", size)
	}
	f := &Footer{}
	var b [FooterSize]byte
	if _, err := r.ReadAt(b[:], size-FooterSize); err != nil {
		return nil, err
	}
	copy(f.sig[:], b[:blockSize])
	copy(f.meta[:], b[blockSize:])
	meta, le := f.meta[:], binary.LittleEndian
	if string(meta[0:8]) != magic {
		return nil, refuse(NotAPackage, "magic", "the file does not end in a package's footer")
	}
	if v := le.Uint16(meta[8:]); v != format {
		return nil, refuse(NotAPackage, "format", "the footer is of format %d; this reader reads format %d", v, format)
	}
	if v := le.Uint16(meta[10:]); v != covered {
		return nil, refuse(Unreadable, "header_len", "the footer covers %d bytes; format %d covers %d", v, format, covered)
	}
	if le.Uint32(meta[covered:]) != crc32.ChecksumIEEE(meta[:covered]) {
		return nil, refuse(Damaged, "crc32", "the metadata is not what its checksum covers")
	}
	for _, r := range reserved {
		if !bytes.Equal(meta[r[0]:r[1]], make([]byte, r[1]-r[0])) {
			return nil, refuse(Unreadable, "reserved", "the reserved bytes %d–%d are not zero", r[0], r[1]-1)
		}
	}
	m := Meta{Model: le.Uint16(meta[12:]), HashID: meta[14], SigID: meta[15], SealID: meta[16],
		PayloadSize: le.Uint64(meta[24:]), BuiltAt: time.Unix(0, int64(le.Uint64(meta[32:]))).UTC()}
	copy(m.Digest[:], meta[40:72])
	copy(m.KeyID[:], meta[72:104])
	for _, fd := range fields {
		v := bytes.TrimRight(meta[fd.at:fd.at+fd.width], "\x00")
		if !utf8.Valid(v) {
			return nil, refuse(Damaged, fd.name, "%s is not UTF-8", fd.name)
		}
		m.setText(fd.name, string(v))
	}
	if m.PayloadSize != uint64(size-FooterSize) {
		return nil, refuse(Damaged, "payload_size", "the payload region is %d bytes and the footer says %d", size-FooterSize, m.PayloadSize)
	}
	f.Meta = m
	return f, nil
}

// Keys is the set of public keys a host trusts, looked up by `key_id`.
type Keys interface {
	Key(id [32]byte) (crypto.PublicKey, bool)
}

// Policy is the host's policy about signatures.
type Policy struct {
	RequireSignature bool
}

// Host is what this host is: the architecture its executables run on, the description models it reads,
// and the Core it will execute.
type Host struct {
	Arch   string
	Models []uint16
	Core   version.Version
}

// Check performs steps 2 and 3 of the order on the footer alone: the signature against the trusted keys
// and the policy, then the architecture, the model and the Core range against the host, and the version.
// An algorithm this host does not know is reported as undecidable before anything is decided.
func (f *Footer) Check(keys Keys, p Policy, h Host) error {
	m := f.Meta
	if m.HashID != HashSHA256 {
		return refuse(Undecidable, "hash_id", "hash_id %d names a digest algorithm this host does not know", m.HashID)
	}
	if m.SigID > SigRSA2048 {
		return refuse(Undecidable, "sig_id", "sig_id %d names a signature algorithm this host does not know", m.SigID)
	}
	if m.SealID > SealAES256GCM {
		return refuse(Undecidable, "seal_id", "seal_id %d names a seal algorithm this host does not know", m.SealID)
	}
	if err := f.verify(keys, p); err != nil {
		return err
	}
	if m.Arch != h.Arch {
		return refuse(Unwanted, "arch", "the package is built for %s and this host is %s", m.Arch, h.Arch)
	}
	if !slices.Contains(h.Models, m.Model) {
		return refuse(Unwanted, "model", "the package is written against model %d, which this host does not read", m.Model)
	}
	r, err := version.ParseRange(m.Core)
	if err != nil {
		return refuse(Invalid, "core", "%v", err)
	}
	if !r.Contains(h.Core) {
		return refuse(Unwanted, "core", "the package declares Core %s and this host executes %s", m.Core, h.Core)
	}
	if _, err := version.Parse(m.Version); err != nil {
		return refuse(Invalid, "version", "%v", err)
	}
	return nil
}

func (f *Footer) verify(keys Keys, p Policy) error {
	m := f.Meta
	if m.SigID == SigNone {
		if p.RequireSignature {
			return refuse(Trust, "sig_id", "the package carries no signature and this host's policy requires one")
		}
		return nil
	}
	pub, ok := keys.Key(m.KeyID)
	if !ok {
		return refuse(Trust, "key_id", "the package is signed by key %s, which this host does not trust", hex.EncodeToString(m.KeyID[:]))
	}
	var good bool
	switch k := pub.(type) {
	case ed25519.PublicKey:
		good = m.SigID == SigEd25519 && ed25519.Verify(k, f.meta[:], f.sig[:ed25519.SignatureSize])
	case *rsa.PublicKey:
		d := sha256.Sum256(f.meta[:])
		good = m.SigID == SigRSA2048 && rsa.VerifyPKCS1v15(k, crypto.SHA256, d[:], f.sig[:]) == nil
	}
	if !good {
		return refuse(Trust, "sig", "the signature over the metadata does not verify against key %s", hex.EncodeToString(m.KeyID[:]))
	}
	return nil
}

// Open performs steps 4 and 5: it reads the payload region, unseals it with the credential where it is
// sealed, and recomputes the digest. Nothing it returns has been used before the digest held.
func (f *Footer) Open(r io.ReaderAt, credential []byte) ([]byte, error) {
	region := make([]byte, f.Meta.PayloadSize)
	if _, err := r.ReadAt(region, 0); err != nil {
		return nil, err
	}
	payload := region
	if f.Meta.SealID == SealAES256GCM {
		if credential == nil {
			return nil, refuse(Seal, "seal_id", "the package is sealed and no credential was given")
		}
		gcm, err := aead(credential)
		if err != nil {
			return nil, refuse(Seal, "seal_id", "%v", err)
		}
		if len(region) < nonceSize+tagSize {
			return nil, refuse(Damaged, "payload_size", "a sealed payload of %d bytes cannot hold a nonce and a tag", len(region))
		}
		payload, err = gcm.Open(nil, region[:nonceSize], region[nonceSize:], nil)
		if err != nil {
			return nil, refuse(Seal, "seal_id", "the seal does not open: the credential is not this package's, or the payload was altered")
		}
	}
	if sha256.Sum256(payload) != f.Meta.Digest {
		return nil, refuse(Digest, "digest", "the payload is not the payload the metadata describes")
	}
	return payload, nil
}
