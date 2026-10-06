package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// BlockSig describes one block of the old file.
type BlockSig struct {
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
	Weak   uint32 `json:"weak"`
	Strong string `json:"strong"` // hex SHA-256 of the block bytes
}

// Signature binds the block list to the whole old file.
type Signature struct {
	BlockSize int64      `json:"blockSize"`
	FileSize  int64      `json:"fileSize"`
	SHA256    string     `json:"sha256"` // hex SHA-256 of the whole old file
	Blocks    []BlockSig `json:"blocks"`
}

// weakChecksum is a rolling checksum over buf, rsync-style:
//
//	a = sum(buf[i])
//	b = sum((len-i) * buf[i])   for i in 0..len-1
//	weak = (b << 16) | (a & 0xffff)
//
// Rolling update when a window of length n slides, dropping xOut and
// appending xIn:
//
//	a' = a - xOut + xIn
//	b' = b - n*xOut + a'
func weakChecksum(buf []byte) uint32 {
	var a, b uint32
	for i, c := range buf {
		a += uint32(c)
		b += uint32(len(buf)-i) * uint32(c)
	}
	return (b << 16) | (a & 0xffff)
}

// BuildSignature reads the old file and splits it into blocks of blockSize
// bytes; the final block may be shorter. An empty file yields no blocks.
func BuildSignature(data []byte, blockSize int) *Signature {
	sum := sha256.Sum256(data)
	sig := &Signature{
		BlockSize: int64(blockSize),
		FileSize:  int64(len(data)),
		SHA256:    hex.EncodeToString(sum[:]),
	}
	for off := 0; off < len(data); off += blockSize {
		end := off + blockSize
		if end > len(data) {
			end = len(data)
		}
		blk := data[off:end]
		strong := sha256.Sum256(blk)
		sig.Blocks = append(sig.Blocks, BlockSig{
			Offset: int64(off),
			Length: len(blk),
			Weak:   weakChecksum(blk),
			Strong: hex.EncodeToString(strong[:]),
		})
	}
	return sig
}

// WriteSignature generates the signature of oldPath and writes it as JSON.
func WriteSignature(oldPath string, blockSize int, outPath string) error {
	if blockSize <= 0 {
		return fmt.Errorf("block size must be a positive integer, got %d", blockSize)
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}
	sig := BuildSignature(data, blockSize)
	buf, err := json.MarshalIndent(sig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, buf, 0o644)
}

// LoadSignature reads and validates a signature file.
func LoadSignature(path string) (*Signature, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sig Signature
	if err := json.Unmarshal(buf, &sig); err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}
	if sig.BlockSize <= 0 {
		return nil, fmt.Errorf("invalid signature: non-positive block size %d", sig.BlockSize)
	}
	var want int64
	for i, b := range sig.Blocks {
		if b.Offset != want || b.Length <= 0 || int64(b.Length) > sig.BlockSize {
			return nil, fmt.Errorf("invalid signature: bad block %d", i)
		}
		want += int64(b.Length)
	}
	if want != sig.FileSize {
		return nil, fmt.Errorf("invalid signature: blocks cover %d bytes, file size is %d", want, sig.FileSize)
	}
	return &sig, nil
}
