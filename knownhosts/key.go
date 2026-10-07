package knownhosts

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// KeyTypeEd25519 is the only supported public key algorithm.
const KeyTypeEd25519 = "ssh-ed25519"

// ed25519KeyLen is the raw Ed25519 public key length in bytes.
const ed25519KeyLen = 32

// Key is a validated Ed25519 public key.
type Key struct {
	Blob        []byte // full SSH wire-format blob
	Fingerprint string // "SHA256:..." of the blob, OpenSSH style
}

// ValidateKey checks that the declared key type is supported and that the
// decoded blob is a well-formed SSH wire-format Ed25519 public key whose
// embedded algorithm name and key length are consistent.
func ValidateKey(keyType string, blob []byte) (*Key, error) {
	if keyType != KeyTypeEd25519 {
		return nil, fmt.Errorf("unsupported key type %q: only %s is supported", keyType, KeyTypeEd25519)
	}
	inner, err := readString(blob)
	if err != nil {
		return nil, fmt.Errorf("corrupt key blob: %v", err)
	}
	if string(inner) != KeyTypeEd25519 {
		return nil, fmt.Errorf("key type mismatch: line declares %q but blob contains %q", keyType, inner)
	}
	rest := blob[4+len(inner):]
	raw, err := readString(rest)
	if err != nil {
		return nil, fmt.Errorf("corrupt key blob: %v", err)
	}
	if len(raw) != ed25519KeyLen {
		return nil, fmt.Errorf("invalid ed25519 key length %d, want %d", len(raw), ed25519KeyLen)
	}
	if len(rest) != 4+len(raw) {
		return nil, fmt.Errorf("trailing %d bytes after ed25519 key material", len(rest)-4-len(raw))
	}
	sum := sha256.Sum256(blob)
	return &Key{
		Blob:        blob,
		Fingerprint: "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]),
	}, nil
}

// readString reads one SSH wire-format string (uint32 length + bytes).
func readString(b []byte) ([]byte, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("truncated length prefix")
	}
	n := binary.BigEndian.Uint32(b[:4])
	if uint64(n) > uint64(len(b)-4) {
		return nil, fmt.Errorf("declared length %d exceeds remaining %d bytes", n, len(b)-4)
	}
	return b[4 : 4+n], nil
}

// SameKey reports whether two validated keys are identical.
func SameKey(a, b *Key) bool {
	return bytes.Equal(a.Blob, b.Blob)
}
