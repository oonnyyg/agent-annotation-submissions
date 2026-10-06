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

// ErrCorruptStore is returned when the on-disk archive fails validation.
var ErrCorruptStore = errors.New("corrupt ledger archive")

const storeFile = "ledger.json"

type envelope struct {
	Checksum string          `json:"checksum"`
	Payload  json.RawMessage `json:"payload"`
}

// Save persists the ledger atomically: a temp file is written, fsynced and
// renamed over the target, so a failed write never leaves a half rotation.
func Save(dir string, l *Ledger) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(l)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	env, err := json.Marshal(envelope{Checksum: hex.EncodeToString(sum[:]), Payload: payload})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ledger-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(env); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, storeFile)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Load reads the ledger from dir. A missing archive yields an empty ledger;
// a corrupt or tampered archive is explicitly rejected with ErrCorruptStore.
func Load(dir string) (*Ledger, error) {
	raw, err := os.ReadFile(filepath.Join(dir, storeFile))
	if errors.Is(err, os.ErrNotExist) {
		return NewLedger(), nil
	}
	if err != nil {
		return nil, err
	}
	var env envelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptStore, err)
	}
	sum := sha256.Sum256(env.Payload)
	if hex.EncodeToString(sum[:]) != env.Checksum {
		return nil, fmt.Errorf("%w: checksum mismatch", ErrCorruptStore)
	}
	var l Ledger
	dec = json.NewDecoder(bytes.NewReader(env.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptStore, err)
	}
	if l.Sessions == nil {
		return nil, fmt.Errorf("%w: missing sessions", ErrCorruptStore)
	}
	l.reindex()
	return &l, nil
}
