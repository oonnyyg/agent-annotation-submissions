
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// archiveFile is the on-disk JSON archive. Integrity is protected by a
// SHA-256 checksum over the canonical encoding of the batch list; any
// corruption or hand-editing makes LoadArchive reject the file.
type archiveFile struct {
	Format   int      `json:"format"`
	Batches  []*Batch `json:"batches"`
	Checksum string   `json:"checksum"`
}

const archiveFormat = 1

func checksumBatches(batches []*Batch) (string, error) {
	data, err := json.Marshal(batches)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// SaveArchive writes the register's applied batches to path atomically.
func SaveArchive(path string, batches []*Batch) error {
	sum, err := checksumBatches(batches)
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(archiveFile{Format: archiveFormat, Batches: batches, Checksum: sum}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadArchive reads an archive, verifies its checksum, and replays the
// batches into a fresh register. Any corruption, format mismatch, or
// invalid batch sequence is an error and yields no register.
func LoadArchive(path string) (*Register, []*Batch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var af archiveFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&af); err != nil {
		return nil, nil, fmt.Errorf("corrupt archive: %v", err)
	}
	if af.Format != archiveFormat {
		return nil, nil, fmt.Errorf("unsupported archive format %d", af.Format)
	}
	sum, err := checksumBatches(af.Batches)
	if err != nil {
		return nil, nil, err
	}
	if sum != af.Checksum {
		return nil, nil, fmt.Errorf("archive checksum mismatch: file is corrupt or tampered")
	}
	reg := NewRegister()
	for i, b := range af.Batches {
		if b == nil {
			return nil, nil, fmt.Errorf("archive batch %d is null", i)
		}
		if err := reg.Apply(b); err != nil {
			return nil, nil, fmt.Errorf("archive batch %d invalid: %v", i, err)
		}
	}
	return reg, af.Batches, nil
}
