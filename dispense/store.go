package dispense

import (
	"encoding/json"
	"fmt"
	"os"
)

// state is the JSON-serializable snapshot of an ATM.
type state struct {
	Physical map[string]int64 `json:"physical"`
	Requests []*Request       `json:"requests"`
}

// Save persists the ATM state to path atomically (temp file + rename).
func (a *ATM) Save(path string) error {
	s := state{
		Physical: map[string]int64{},
		Requests: make([]*Request, 0, len(a.Requests)),
	}
	for d, n := range a.Physical {
		s.Physical[fmt.Sprint(d)] = n
	}
	for _, r := range a.Requests {
		s.Requests = append(s.Requests, r)
	}
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

// LoadATM restores an ATM from path. A missing file yields an empty ATM.
func LoadATM(path string) (*ATM, error) {
	a := NewATM()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return a, nil
		}
		return nil, err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("corrupt state file: %w", err)
	}
	for ds, n := range s.Physical {
		var d int64
		if _, err := fmt.Sscan(ds, &d); err != nil {
			return nil, fmt.Errorf("invalid denomination key %q", ds)
		}
		a.Physical[d] = n
	}
	for _, r := range s.Requests {
		a.Requests[r.ID] = r
		if r.Status == StatusReserved {
			for ds, n := range r.Plan {
				var d int64
				fmt.Sscan(ds, &d)
				a.Reserved[d] += n
			}
		}
	}
	return a, nil
}
