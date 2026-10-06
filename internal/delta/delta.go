package delta

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	SigFormat   = "blockdelta-sig-v1"
	DeltaFormat = "blockdelta-delta-v1"
)

// BlockSig describes one block of the old file.
type BlockSig struct {
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
	Weak   uint32 `json:"weak"`
	Strong string `json:"strong"`
}

// Signature is the JSON signature of the old (basis) file.
type Signature struct {
	Format    string     `json:"format"`
	BlockSize int        `json:"blockSize"`
	FileSize  int64      `json:"fileSize"`
	SHA256    string     `json:"sha256"`
	Blocks    []BlockSig `json:"blocks"`
}

// Op is one delta instruction: either "copy" from the old file or "literal".
type Op struct {
	Op     string `json:"op"`
	Offset int64  `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
	Data   []byte `json:"data,omitempty"`
}

// Delta is the JSON delta binding old-file identity and new-file identity.
type Delta struct {
	Format       string `json:"format"`
	SourceSize   int64  `json:"sourceSize"`
	SourceSHA256 string `json:"sourceSHA256"`
	TargetSize   int64  `json:"targetSize"`
	TargetSHA256 string `json:"targetSHA256"`
	Ops          []Op   `json:"ops"`
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// weakSum computes the rolling weak checksum of b:
// a = sum(b[i]) mod 2^16, c = sum((n-i)*b[i]) mod 2^16, weak = a | c<<16.
func weakSum(b []byte) uint32 {
	var a, c uint32
	n := len(b)
	for i, v := range b {
		a += uint32(v)
		c += uint32(n-i) * uint32(v)
	}
	return (a & 0xffff) | ((c & 0xffff) << 16)
}

// roller maintains the weak checksum of a fixed-size sliding window.
type roller struct {
	n int
	a uint32
	c uint32
}

func newRoller(window []byte) *roller {
	r := &roller{n: len(window)}
	for i, v := range window {
		r.a += uint32(v)
		r.c += uint32(r.n-i) * uint32(v)
	}
	return r
}

func (r *roller) sum() uint32 {
	return (r.a & 0xffff) | ((r.c & 0xffff) << 16)
}

// roll slides the window one byte: out leaves, in enters.
func (r *roller) roll(out, in byte) {
	r.a = r.a - uint32(out) + uint32(in)
	r.c = r.c - uint32(r.n)*uint32(out) + r.a
}
