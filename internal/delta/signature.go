package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// BuildSignature reads the old file from r and produces its signature.
// blockSize must be a positive integer; the last block may be shorter.
func BuildSignature(r io.Reader, blockSize int) (*Signature, error) {
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be a positive integer, got %d", blockSize)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read basis file: %w", err)
	}
	sig := &Signature{
		Format:    SigFormat,
		BlockSize: blockSize,
		FileSize:  int64(len(data)),
		SHA256:    sha256Hex(data),
		Blocks:    []BlockSig{},
	}
	for off := 0; off < len(data); off += blockSize {
		end := off + blockSize
		if end > len(data) {
			end = len(data)
		}
		blk := data[off:end]
		sig.Blocks = append(sig.Blocks, BlockSig{
			Offset: int64(off),
			Length: len(blk),
			Weak:   weakSum(blk),
			Strong: sha256Hex(blk),
		})
	}
	return sig, nil
}

// Validate checks the internal consistency of a signature.
func (s *Signature) Validate() error {
	if s.Format != SigFormat {
		return fmt.Errorf("unsupported signature format %q", s.Format)
	}
	if s.BlockSize <= 0 {
		return fmt.Errorf("invalid block size %d", s.BlockSize)
	}
	if s.FileSize < 0 {
		return fmt.Errorf("invalid file size %d", s.FileSize)
	}
	var off int64
	for i, b := range s.Blocks {
		if b.Offset != off {
			return fmt.Errorf("block %d: offset %d, want %d", i, b.Offset, off)
		}
		if b.Length <= 0 || b.Length > s.BlockSize {
			return fmt.Errorf("block %d: invalid length %d", i, b.Length)
		}
		if i < len(s.Blocks)-1 && b.Length != s.BlockSize {
			return fmt.Errorf("block %d: non-final block shorter than block size", i)
		}
		if len(b.Strong) != sha256.Size*2 {
			return fmt.Errorf("block %d: invalid strong digest", i)
		}
		if _, err := hex.DecodeString(b.Strong); err != nil {
			return fmt.Errorf("block %d: bad strong digest: %w", i, err)
		}
		off += int64(b.Length)
	}
	if off != s.FileSize {
		return fmt.Errorf("blocks cover %d bytes, file size is %d", off, s.FileSize)
	}
	if len(s.SHA256) != sha256.Size*2 {
		return fmt.Errorf("invalid file digest")
	}
	return nil
}

// VerifyBasis checks that data matches the signature's file identity.
func (s *Signature) VerifyBasis(data []byte) error {
	if int64(len(data)) != s.FileSize {
		return fmt.Errorf("basis size %d does not match signature size %d", len(data), s.FileSize)
	}
	if got := sha256Hex(data); got != s.SHA256 {
		return fmt.Errorf("basis SHA-256 %s does not match signature %s", got, s.SHA256)
	}
	return nil
}
