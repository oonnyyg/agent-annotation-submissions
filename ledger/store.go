package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const storeFile = "ledger.json"

var ErrCorruptStore = errors.New("corrupt ledger archive")

type envelope struct {
	Checksum string          `json:"checksum"`
	Ledger   json.RawMessage `json:"ledger"`
}

// Save atomically persists the ledger into dir. A checksum-protected envelope
// is written to a temp file and renamed over the target, so a failed write
// never leaves a half-written archive.
func Save(dir string, l *Ledger) error {
	payload, err := json.Marshal(l)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	env, err := json.Marshal(envelope{
		Checksum: hex.EncodeToString(sum[:]),
		Ledger:   payload,
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, storeFile+".tmp")
	if err := os.WriteFile(tmp, env, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, storeFile))
}

// Load reads the ledger from dir. A missing archive yields a fresh ledger;
// a damaged or tampered archive is explicitly rejected.
func Load(dir string) (*Ledger, error) {
	data, err := os.ReadFile(filepath.Join(dir, storeFile))
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	var env envelope
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptStore, err)
	}
	sum := sha256.Sum256(env.Ledger)
	if hex.EncodeToString(sum[:]) != env.Checksum {
		return nil, fmt.Errorf("%w: checksum mismatch", ErrCorruptStore)
	}
	var l Ledger
	dec = json.NewDecoder(bytes.NewReader(env.Ledger))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptStore, err)
	}
	if l.Sessions == nil || l.Tokens == nil {
		return nil, fmt.Errorf("%w: nil maps", ErrCorruptStore)
	}
	for id, t := range l.Tokens {
		if t.ID != id {
			return nil, fmt.Errorf("%w: token key mismatch", ErrCorruptStore)
		}
		if _, ok := l.Sessions[t.SessionID]; !ok {
			return nil, fmt.Errorf("%w: token %s references unknown session", ErrCorruptStore, id)
		}
		if t.ParentID != "" {
			if _, ok := l.Tokens[t.ParentID]; !ok {
				return nil, fmt.Errorf("%w: token %s references unknown parent", ErrCorruptStore, id)
			}
		}
	}
	return &l, nil
}
