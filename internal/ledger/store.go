package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const stateFile = "state.json"

// Load reads the persisted state from dir, or returns a fresh state
// when the directory has no state file yet.
func Load(dir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return NewState(), nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Batches == nil {
		st.Batches = map[string]*Batch{}
	}
	if st.Requests == nil {
		st.Requests = map[string]*Request{}
	}
	return &st, nil
}

// Save atomically writes the state into dir.
func Save(dir string, st *State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, stateFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, stateFile))
}
