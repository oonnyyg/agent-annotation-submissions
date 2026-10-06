package delta

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Validate checks the delta's structural consistency against the signature.
func (d *Delta) Validate(sig *Signature) error {
	if d.Format != DeltaFormat {
		return fmt.Errorf("unsupported delta format %q", d.Format)
	}
	if d.SourceSize != sig.FileSize || d.SourceSHA256 != sig.SHA256 {
		return fmt.Errorf("delta was built for a different basis file")
	}
	if d.TargetSize < 0 {
		return fmt.Errorf("invalid target size %d", d.TargetSize)
	}
	var out int64
	for i, op := range d.Ops {
		switch op.Op {
		case "copy":
			if op.Length <= 0 || op.Offset < 0 || op.Offset+int64(op.Length) > sig.FileSize {
				return fmt.Errorf("op %d: copy out of bounds (offset %d, length %d, basis %d)",
					i, op.Offset, op.Length, sig.FileSize)
			}
			out += int64(op.Length)
		case "literal":
			if len(op.Data) == 0 {
				return fmt.Errorf("op %d: empty literal", i)
			}
			out += int64(len(op.Data))
		default:
			return fmt.Errorf("op %d: unknown op %q", i, op.Op)
		}
	}
	if out != d.TargetSize {
		return fmt.Errorf("ops produce %d bytes, target size is %d", out, d.TargetSize)
	}
	return nil
}

// Rebuild applies the delta to basis (old file contents) in memory and
// verifies the reconstructed target digest.
func Rebuild(d *Delta, basis []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(int(d.TargetSize))
	for i, op := range d.Ops {
		switch op.Op {
		case "copy":
			buf.Write(basis[op.Offset : op.Offset+int64(op.Length)])
		case "literal":
			buf.Write(op.Data)
		default:
			return nil, fmt.Errorf("op %d: unknown op %q", i, op.Op)
		}
	}
	out := buf.Bytes()
	if int64(len(out)) != d.TargetSize {
		return nil, fmt.Errorf("rebuilt %d bytes, want %d", len(out), d.TargetSize)
	}
	sum := sha256.Sum256(out)
	if got := hex.EncodeToString(sum[:]); got != d.TargetSHA256 {
		return nil, fmt.Errorf("rebuilt SHA-256 %s does not match delta %s", got, d.TargetSHA256)
	}
	return out, nil
}

// ApplyToFile rebuilds the target and atomically replaces outPath only after
// full verification. outPath must differ from oldPath. On any failure the
// existing target is preserved and the old file is never modified.
func ApplyToFile(d *Delta, sig *Signature, oldPath, outPath string) error {
	oldAbs, err := filepath.Abs(oldPath)
	if err != nil {
		return err
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if oldAbs == outAbs {
		return fmt.Errorf("target path must differ from the old file path")
	}
	if err := d.Validate(sig); err != nil {
		return fmt.Errorf("invalid delta: %w", err)
	}
	basis, err := os.ReadFile(oldAbs)
	if err != nil {
		return fmt.Errorf("read old file: %w", err)
	}
	if err := sig.VerifyBasis(basis); err != nil {
		return fmt.Errorf("old file identity check failed: %w", err)
	}
	out, err := Rebuild(d, basis)
	if err != nil {
		return err
	}

	dir := filepath.Dir(outAbs)
	tmp, err := os.CreateTemp(dir, ".blockdelta-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	// Verify the temp file once more before replacing the target.
	written, err := os.ReadFile(tmpName)
	if err != nil {
		return fmt.Errorf("re-read temp file: %w", err)
	}
	if !bytes.Equal(written, out) {
		return fmt.Errorf("temp file verification failed")
	}
	if err := os.Rename(tmpName, outAbs); err != nil {
		// Windows cannot rename over an existing file; remove then retry.
		if rmErr := os.Remove(outAbs); rmErr != nil {
			return fmt.Errorf("replace target: %w", err)
		}
		if err2 := os.Rename(tmpName, outAbs); err2 != nil {
			return fmt.Errorf("replace target: %w", err2)
		}
	}
	return nil
}
