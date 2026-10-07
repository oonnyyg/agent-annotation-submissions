
package orset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Load opens the persisted state at path. If the file does not exist, a new
// state is created for the given replica identity. If the file exists, the
// stored identity wins and must match replica when replica is non-empty.
func Load(path, replica string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(replica)
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("orset: corrupt state file: %w", err)
	}
	if s.Replica == "" || s.Adds == nil {
		return nil, fmt.Errorf("orset: state file missing identity or adds")
	}
	if replica != "" && replica != s.Replica {
		return nil, fmt.Errorf("orset: state belongs to replica %q, not %q", s.Replica, replica)
	}
	return &s, nil
}

// Save atomically persists the state to path (write temp file, then rename),
// so identity and sequence counter survive restarts.
func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
