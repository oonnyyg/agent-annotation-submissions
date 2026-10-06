package delta

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Op is one delta instruction: either a copy from the old file or a literal.
type Op struct {
	Op     string `json:"op"`               // "copy" or "literal"
	Offset int64  `json:"offset,omitempty"` // copy: offset in old file
	Length int64  `json:"length,omitempty"` // copy: byte count
	Data   []byte `json:"data,omitempty"`   // literal: raw bytes (base64 in JSON)
}

// Delta binds the operation list to both file identities.
type Delta struct {
	BlockSize int64  `json:"blockSize"`
	OldSize   int64  `json:"oldSize"`
	OldSHA256 string `json:"oldSHA256"`
	NewSize   int64  `json:"newSize"`
	NewSHA256 string `json:"newSHA256"`
	Ops       []Op   `json:"ops"`
}

// Stats reports how many bytes were reused vs sent literally.
type Stats struct {
	Reused  int64
	Literal int64
}

// roller maintains the rolling weak checksum over a fixed-size window.
type roller struct {
	n    int // window size
	a, b uint32
}

func newRoller(window []byte) *roller {
	r := &roller{n: len(window)}
	for i, c := range window {
		r.a += uint32(c)
		r.b += uint32(len(window)-i) * uint32(c)
	}
	return r
}

func (r *roller) value() uint32 { return (r.b << 16) | (r.a & 0xffff) }

// roll slides the window: drop xOut, append xIn.
func (r *roller) roll(xOut, xIn byte) {
	r.a = r.a - uint32(xOut) + uint32(xIn)
	r.b = r.b - uint32(r.n)*uint32(xOut) + r.a
}

// matchAt returns the best block matching data[pos:pos+length]: among equal
// content matches the smallest old offset wins (blocks are indexed in offset
// order, so the first strong-hash hit is the smallest offset).
func matchAt(sig *Signature, byWeak map[uint32][]int, window []byte, weak uint32) (BlockSig, bool) {
	for _, idx := range byWeak[weak] {
		blk := sig.Blocks[idx]
		if blk.Length != len(window) {
			continue
		}
		sum := sha256.Sum256(window)
		if hex.EncodeToString(sum[:]) == blk.Strong {
			return blk, true
		}
	}
	return BlockSig{}, false
}

// BuildDelta generates the op list for newData using only the signature;
// the old file is never read. Scanning is byte-by-byte (any offset, not just
// block-aligned). At each position the longest match wins, ties break by
// smallest old offset. Unmatched bytes accumulate into merged literals.
func BuildDelta(sig *Signature, newData []byte) *Delta {
	bs := int(sig.BlockSize)
	tail := 0
	if n := len(sig.Blocks); n > 0 && int64(sig.Blocks[n-1].Length) < sig.BlockSize {
		tail = sig.Blocks[n-1].Length
	}
	byWeak := make(map[uint32][]int)
	for i, b := range sig.Blocks {
		byWeak[b.Weak] = append(byWeak[b.Weak], i)
	}

	d := &Delta{
		BlockSize: sig.BlockSize,
		OldSize:   sig.FileSize,
		OldSHA256: sig.SHA256,
		NewSize:   int64(len(newData)),
	}
	sum := sha256.Sum256(newData)
	d.NewSHA256 = hex.EncodeToString(sum[:])

	var lit []byte
	flushLit := func() {
		if len(lit) > 0 {
			d.Ops = append(d.Ops, Op{Op: "literal", Data: lit})
			lit = nil
		}
	}
	emitCopy := func(blk BlockSig) {
		// Merge with a preceding contiguous copy of the following block region.
		if n := len(d.Ops); n > 0 {
			last := &d.Ops[n-1]
			if last.Op == "copy" && last.Offset+last.Length == blk.Offset {
				last.Length += int64(blk.Length)
				return
			}
		}
		d.Ops = append(d.Ops, Op{Op: "copy", Offset: blk.Offset, Length: int64(blk.Length)})
	}

	n := len(newData)
	var full, tl *roller
	if n >= bs && bs > 0 {
		full = newRoller(newData[:bs])
	}
	if tail > 0 && n >= tail {
		tl = newRoller(newData[:tail])
	}
	for pos := 0; pos < n; {
		best, bestLen, ok := BlockSig{}, 0, false
		if full != nil && pos+bs <= n {
			if blk, hit := matchAt(sig, byWeak, newData[pos:pos+bs], full.value()); hit {
				best, bestLen, ok = blk, bs, true
			}
		}
		if tl != nil && pos+tail <= n && tail > bestLen {
			if blk, hit := matchAt(sig, byWeak, newData[pos:pos+tail], tl.value()); hit {
				best, bestLen, ok = blk, tail, true
			}
		}
		if ok {
			flushLit()
			emitCopy(best)
			advance(pos, bestLen, newData, full, tl, bs, tail)
			pos += bestLen
			continue
		}
		lit = append(lit, newData[pos])
		advance(pos, 1, newData, full, tl, bs, tail)
		pos++
	}
	flushLit()
	return d
}

// advance rolls both windows forward by step bytes starting at pos.
func advance(pos, step int, data []byte, full, tl *roller, bs, tail int) {
	for k := 0; k < step; k++ {
		i := pos + k
		if full != nil && i+bs < len(data) {
			full.roll(data[i], data[i+bs])
		}
		if tl != nil && i+tail < len(data) {
			tl.roll(data[i], data[i+tail])
		}
	}
}

// WriteDelta builds the delta from a signature and the new file.
func WriteDelta(sigPath, newPath, outPath string) (Stats, error) {
	var st Stats
	sig, err := LoadSignature(sigPath)
	if err != nil {
		return st, err
	}
	newData, err := os.ReadFile(newPath)
	if err != nil {
		return st, err
	}
	d := BuildDelta(sig, newData)
	for _, op := range d.Ops {
		if op.Op == "copy" {
			st.Reused += op.Length
		} else {
			st.Literal += int64(len(op.Data))
		}
	}
	buf, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return st, err
	}
	return st, os.WriteFile(outPath, buf, 0o644)
}

// LoadDelta reads and structurally validates a delta file.
func LoadDelta(path string) (*Delta, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Delta
	if err := json.Unmarshal(buf, &d); err != nil {
		return nil, fmt.Errorf("invalid delta: %w", err)
	}
	if d.BlockSize <= 0 || d.OldSize < 0 || d.NewSize < 0 {
		return nil, fmt.Errorf("invalid delta: bad sizes")
	}
	var total int64
	for i, op := range d.Ops {
		switch op.Op {
		case "copy":
			if op.Length <= 0 || op.Offset < 0 || op.Offset+op.Length > d.OldSize {
				return nil, fmt.Errorf("invalid delta: copy op %d out of bounds", i)
			}
			total += op.Length
		case "literal":
			if len(op.Data) == 0 {
				return nil, fmt.Errorf("invalid delta: empty literal op %d", i)
			}
			total += int64(len(op.Data))
		default:
			return nil, fmt.Errorf("invalid delta: unknown op %q at %d", op.Op, i)
		}
	}
	if total != d.NewSize {
		return nil, fmt.Errorf("invalid delta: ops produce %d bytes, expected %d", total, d.NewSize)
	}
	return &d, nil
}

// verifyOld checks the old file matches the identity recorded in the delta.
func verifyOld(oldPath string, d *Delta) ([]byte, error) {
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != d.OldSize || hex.EncodeToString(sum[:]) != d.OldSHA256 {
		return nil, fmt.Errorf("old file %s does not match the delta's expected basis", oldPath)
	}
	return data, nil
}

// Apply rebuilds the new file from oldPath + delta and atomically replaces
// targetPath only after full reconstruction and checksum verification.
// Any failure leaves the existing target and the old file untouched.
func Apply(oldPath, deltaPath, targetPath string) (Stats, error) {
	var st Stats
	oldAbs, err := canonicalPath(oldPath)
	if err != nil {
		return st, err
	}
	targetAbs, err := canonicalPath(targetPath)
	if err != nil {
		return st, err
	}
	if oldAbs == targetAbs {
		return st, fmt.Errorf("target path must differ from the old file path")
	}
	d, err := LoadDelta(deltaPath)
	if err != nil {
		return st, err
	}
	oldData, err := verifyOld(oldPath, d)
	if err != nil {
		return st, err
	}

	var out bytes.Buffer
	for _, op := range d.Ops {
		switch op.Op {
		case "copy":
			out.Write(oldData[op.Offset : op.Offset+op.Length])
			st.Reused += op.Length
		case "literal":
			out.Write(op.Data)
			st.Literal += int64(len(op.Data))
		}
	}
	result := out.Bytes()
	sum := sha256.Sum256(result)
	if int64(len(result)) != d.NewSize || hex.EncodeToString(sum[:]) != d.NewSHA256 {
		return st, fmt.Errorf("reconstructed file failed verification")
	}

	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".blockdelta-*")
	if err != nil {
		return st, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(result); err != nil {
		tmp.Close()
		return st, err
	}
	if err := tmp.Close(); err != nil {
		return st, err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return st, fmt.Errorf("replacing target: %w", err)
	}
	return st, nil
}

func canonicalPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
