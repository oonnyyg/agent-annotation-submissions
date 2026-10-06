package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// LoadInput reads stations and jobs JSON files and validates them.
func LoadInput(stationsPath, jobsPath string) (*Input, error) {
	var in Input
	if err := readJSON(stationsPath, &in.Stations); err != nil {
		return nil, fmt.Errorf("stations: %w", err)
	}
	if err := readJSON(jobsPath, &in.Jobs); err != nil {
		return nil, fmt.Errorf("jobs: %w", err)
	}
	if err := Validate(&in); err != nil {
		return nil, err
	}
	return &in, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Validate rejects malformed instances. IDs are compared lexicographically.
func Validate(in *Input) error {
	seenStation := map[string]bool{}
	for _, s := range in.Stations {
		if s.ID == "" {
			return fmt.Errorf("station with empty id")
		}
		if seenStation[s.ID] {
			return fmt.Errorf("duplicate station id %q", s.ID)
		}
		seenStation[s.ID] = true
		if s.Buffer < 0 {
			return fmt.Errorf("station %q: negative buffer %d", s.ID, s.Buffer)
		}
	}
	seenJob := map[string]bool{}
	for _, j := range in.Jobs {
		if j.ID == "" {
			return fmt.Errorf("job with empty id")
		}
		if seenJob[j.ID] {
			return fmt.Errorf("duplicate job id %q", j.ID)
		}
		seenJob[j.ID] = true
		if j.Release < 0 {
			return fmt.Errorf("job %q: negative release %d", j.ID, j.Release)
		}
		if len(j.Route) == 0 {
			return fmt.Errorf("job %q: empty route", j.ID)
		}
		for i, op := range j.Route {
			if !seenStation[op.Station] {
				return fmt.Errorf("job %q op %d: unknown station %q", j.ID, i, op.Station)
			}
			if op.Duration <= 0 {
				return fmt.Errorf("job %q op %d: non-positive duration %d", j.ID, i, op.Duration)
			}
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
