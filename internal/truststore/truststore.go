// Package truststore reads the file a host keeps its trust policy in: whether a package must be signed,
// the set of keys trusted, and where the credential that opens a sealed package is kept.
//
// The file is the system's: it is required to belong to root and to be writable by nobody else, and it is
// read rather than provisioned. Every key is named by the digest of its own public key, which is what a
// package's footer carries. An unknown key is refused at any level. A variable in the manager's
// environment may tighten the policy and has no spelling that loosens it.
package truststore

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"syscall"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/pkgfile"
)

// Path is where a host keeps the file.
const Path = "/etc/yoke/packages.conf"

// Tighten is the variable that makes a host require a signature where the file did not.
const Tighten = "YOKE_REQUIRE_SIGNATURE"

// credentialSize is the length of an AES-256 key.
const credentialSize = 32

// root is the account the file and the credential must belong to.
var root uint32 = 0

// Key is one key the host trusts.
type Key struct {
	ID        [32]byte
	Label     string
	Algorithm string
	Public    crypto.PublicKey
}

// Store is the file, read.
type Store struct {
	require  bool
	keys     map[[32]byte]Key
	sealPath string
}

type document struct {
	RequireSignature bool `yaml:"require_signature"`
	Keys             []struct {
		ID        string `yaml:"id"`
		Label     string `yaml:"label"`
		Algorithm string `yaml:"algorithm"`
		Public    string `yaml:"public"`
	} `yaml:"keys"`
	Seal struct {
		Credential string `yaml:"credential"`
	} `yaml:"seal"`
}

// Load reads the file at path, after requiring that it belongs to root and that nobody else may write it.
// getenv is the manager's environment, read for the variable that tightens the policy.
func Load(path string, getenv func(string) string) (*Store, error) {
	if err := owned(path, 0o022, "writable by anybody but its owner"); err != nil {
		return nil, fmt.Errorf("%s is refused as a policy file: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d document
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s := &Store{require: d.RequireSignature || getenv(Tighten) == "1", keys: map[[32]byte]Key{}, sealPath: d.Seal.Credential}
	for i, r := range d.Keys {
		at := fmt.Sprintf("%s: keys[%d]", path, i)
		der, err := base64.StdEncoding.DecodeString(r.Public)
		if err != nil {
			return nil, fmt.Errorf("%s: public is not base64: %w", at, err)
		}
		pub, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return nil, fmt.Errorf("%s: public is not a public key: %w", at, err)
		}
		if err := matches(r.Algorithm, pub); err != nil {
			return nil, fmt.Errorf("%s: %w", at, err)
		}
		id := pkgfile.KeyID(pub)
		if r.ID != hex.EncodeToString(id[:]) {
			return nil, fmt.Errorf("%s: id %s is not the digest of its own public key, %x", at, r.ID, id)
		}
		s.keys[id] = Key{ID: id, Label: r.Label, Algorithm: r.Algorithm, Public: pub}
	}
	return s, nil
}

// matches requires the public key to be one of the algorithm the row names.
func matches(algorithm string, pub crypto.PublicKey) error {
	switch algorithm {
	case "ed25519":
		if _, ok := pub.(ed25519.PublicKey); !ok {
			return fmt.Errorf("algorithm ed25519 names a key of type %T", pub)
		}
	case "rsa-2048":
		k, ok := pub.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("algorithm rsa-2048 names a key of type %T", pub)
		}
		if k.N.BitLen() != 2048 {
			return fmt.Errorf("algorithm rsa-2048 names a key of %d bits", k.N.BitLen())
		}
	default:
		return fmt.Errorf("algorithm %q is neither ed25519 nor rsa-2048", algorithm)
	}
	return nil
}

// owned requires path to belong to root and to carry none of the forbidden permission bits.
func owned(path string, forbidden os.FileMode, says string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("its owner cannot be read")
	}
	if st.Uid != root {
		return fmt.Errorf("it is owned by account %d, not by root", st.Uid)
	}
	if mode := info.Mode().Perm(); mode&forbidden != 0 {
		return fmt.Errorf("its mode %#o is %s", mode, says)
	}
	return nil
}

// Empty is the store of a host that keeps no file: no key trusted, no signature required unless the
// environment tightens it, and the credential supplied per invocation.
func Empty(getenv func(string) string) *Store {
	return &Store{require: getenv(Tighten) == "1", keys: map[[32]byte]Key{}}
}

// Key returns the trusted key whose digest is id.
func (s *Store) Key(id [32]byte) (crypto.PublicKey, bool) {
	k, ok := s.keys[id]
	return k.Public, ok
}

// Label is the label of the key whose digest is id, for people; it is compared with nothing.
func (s *Store) Label(id [32]byte) string { return s.keys[id].Label }

// Policy is the host's policy about signatures, tightened by the environment where it was.
func (s *Store) Policy() pkgfile.Policy { return pkgfile.Policy{RequireSignature: s.require} }

// SealPath is where the credential is kept at rest, or empty where it is supplied per invocation.
func (s *Store) SealPath() string { return s.sealPath }

// Credential is the credential that opens a sealed package: read from its path where the file names one,
// which must be root's and readable by nobody else, and otherwise the one the invocation supplied, if any.
// Either is one line of base64 holding 32 bytes.
func (s *Store) Credential(supplied string) ([]byte, error) {
	text := supplied
	if s.sealPath != "" {
		if err := owned(s.sealPath, 0o077, "readable or writable by anybody but its owner"); err != nil {
			return nil, fmt.Errorf("the seal credential %s is refused: %w", s.sealPath, err)
		}
		data, err := os.ReadFile(s.sealPath)
		if err != nil {
			return nil, err
		}
		text = string(data)
	}
	if text == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(string(bytes.TrimRight([]byte(text), " \t\r\n")))
	if err != nil {
		return nil, fmt.Errorf("the seal credential is not base64: %w", err)
	}
	if len(key) != credentialSize {
		return nil, fmt.Errorf("the seal credential is %d bytes; it must be %d", len(key), credentialSize)
	}
	return key, nil
}

// Weaker names the weaker arrangements the file declares.
func (s *Store) Weaker() []string {
	if s.sealPath != "" {
		return []string{"seal.credential.at_rest"}
	}
	return nil
}
