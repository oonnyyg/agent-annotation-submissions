package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// archive is the on-disk JSON form of a Register. The checksum covers the
// canonical encoding of payload so a corrupted archive is rejected.
type archive struct {
	Version  int             `json:"version"`
	Payload  json.RawMessage `json:"payload"`
	Checksum string          `json:"checksum"`
}

type payload struct {
	Batches []Batch `json:"batches"`
}

const archiveVersion = 1

// Save writes the register to path as a checksummed JSON archive.
func (r *Register) Save(path string) error {
	p, err := json.Marshal(payload{Batches: r.batches})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(p)
	a := archive{Version: archiveVersion, Payload: p, Checksum: hex.EncodeToString(sum[:])}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads a JSON archive, verifying its structure and checksum.
// A corrupted or tampered archive is rejected.
func Load(path string) (*Register, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a archive
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("corrupt archive: %w", err)
	}
	if a.Version != archiveVersion {
		return nil, fmt.Errorf("unsupported archive version %d", a.Version)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, a.Payload); err != nil {
		return nil, fmt.Errorf("corrupt archive payload: %w", err)
	}
	sum := sha256.Sum256(compact.Bytes())
	if hex.EncodeToString(sum[:]) != a.Checksum {
		return nil, errors.New("corrupt archive: checksum mismatch")
	}
	var p payload
	dec = json.NewDecoder(bytes.NewReader(a.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("corrupt archive payload: %w", err)
	}
	// Re-apply batches through Submit so a tampered-but-checksummed file
	// with invalid history is still rejected.
	reg := NewRegister()
	for _, b := range p.Batches {
		if err := reg.Submit(b); err != nil {
			return nil, fmt.Errorf("archive history invalid: %w", err)
		}
	}
	return reg, nil
}
