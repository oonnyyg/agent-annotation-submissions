package split

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Run splits r into records and publishes them as one batch directory.
//
// Records are first written to a temporary staging directory next to
// outDir. Only after the whole input parses cleanly is the batch moved
// to outDir, which must not exist yet (existing directories are never
// overwritten). Any failure removes the staging directory, so a broken
// document tail never leaves a half-published batch behind.
//
// The batch contains record-000001.xml ... and an index.txt with the
// record count plus each file's size and SHA-256 digest.
func Run(r io.Reader, path []Name, outDir string, maxRecord int64) (int, error) {
	parent := filepath.Dir(outDir)
	stage, err := os.MkdirTemp(parent, ".staging-*")
	if err != nil {
		return 0, fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stage) // no-op once the batch is published

	type entry struct {
		name string
		size int64
		sum  [32]byte
	}
	var index []entry

	sp := &Splitter{
		Path:      path,
		MaxRecord: maxRecord,
		OnRecord: func(data []byte) error {
			name := fmt.Sprintf("record-%06d.xml", len(index)+1)
			if err := os.WriteFile(filepath.Join(stage, name), data, 0o644); err != nil {
				return fmt.Errorf("stage record: %w", err)
			}
			index = append(index, entry{name, int64(len(data)), sha256.Sum256(data)})
			return nil
		},
	}
	n, err := sp.Split(r)
	if err != nil {
		return 0, err // staging dir removed by deferred RemoveAll
	}

	var idx []byte
	idx = append(idx, fmt.Sprintf("count: %d\n", n)...)
	for _, e := range index {
		idx = append(idx, fmt.Sprintf("%s  bytes=%d  sha256=%x\n", e.name, e.size, e.sum)...)
	}
	if err := os.WriteFile(filepath.Join(stage, "index.txt"), idx, 0o644); err != nil {
		return n, fmt.Errorf("write index: %w", err)
	}

	if err := os.Mkdir(outDir, 0o755); err != nil {
		return n, fmt.Errorf("output directory %q cannot be created (already exists?): %w", outDir, err)
	}
	for _, e := range index {
		if err := os.Rename(filepath.Join(stage, e.name), filepath.Join(outDir, e.name)); err != nil {
			return n, fmt.Errorf("publish %s: %w", e.name, err)
		}
	}
	if err := os.Rename(filepath.Join(stage, "index.txt"), filepath.Join(outDir, "index.txt")); err != nil {
		return n, fmt.Errorf("publish index: %w", err)
	}
	return n, nil
}
