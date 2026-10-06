package workshop

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Op is one step of a job route: a station visit with a processing duration.
type Op struct {
	Station  string `json:"station"`
	Duration int    `json:"duration"`
}

// Job is a work order with a release time and an ordered route.
type Job struct {
	ID      string `json:"id"`
	Release int    `json:"release"`
	Route   []Op   `json:"route"`
}

// Station has one machine and a waiting buffer that excludes the part
// currently on the machine.
type Station struct {
	ID     string `json:"id"`
	Buffer int    `json:"buffer"`
}

// Input is the top-level JSON document consumed by the tool.
type Input struct {
	Stations []Station `json:"stations"`
	Jobs     []Job     `json:"jobs"`
}

// LoadInput decodes and validates the workshop description.
func LoadInput(r io.Reader) (*Input, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var in Input
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("decode input: %w", err)
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	return &in, nil
}

// Validate enforces the structural rules of the workshop.
func (in *Input) Validate() error {
	seenStations := map[string]bool{}
	for _, s := range in.Stations {
		if s.ID == "" {
			return fmt.Errorf("station with empty id")
		}
		if seenStations[s.ID] {
			return fmt.Errorf("duplicate station id %q", s.ID)
		}
		seenStations[s.ID] = true
		if s.Buffer < 0 {
			return fmt.Errorf("station %q: negative buffer capacity %d", s.ID, s.Buffer)
		}
	}
	seenJobs := map[string]bool{}
	for _, j := range in.Jobs {
		if j.ID == "" {
			return fmt.Errorf("job with empty id")
		}
		if seenJobs[j.ID] {
			return fmt.Errorf("duplicate job id %q", j.ID)
		}
		seenJobs[j.ID] = true
		if j.Release < 0 {
			return fmt.Errorf("job %q: negative release time %d", j.ID, j.Release)
		}
		if len(j.Route) == 0 {
			return fmt.Errorf("job %q: empty route", j.ID)
		}
		for i, op := range j.Route {
			if !seenStations[op.Station] {
				return fmt.Errorf("job %q op %d: unknown station %q", j.ID, i, op.Station)
			}
			if op.Duration <= 0 {
				return fmt.Errorf("job %q op %d: non-positive duration %d", j.ID, i, op.Duration)
			}
		}
	}
	return nil
}

// SortedStationIDs returns station ids in lexicographic order.
func (in *Input) SortedStationIDs() []string {
	ids := make([]string, 0, len(in.Stations))
	for _, s := range in.Stations {
		ids = append(ids, s.ID)
	}
	sort.Strings(ids)
	return ids
}

// SortedJobIDs returns job ids in lexicographic order.
func (in *Input) SortedJobIDs() []string {
	ids := make([]string, 0, len(in.Jobs))
	for _, j := range in.Jobs {
		ids = append(ids, j.ID)
	}
	sort.Strings(ids)
	return ids
}
