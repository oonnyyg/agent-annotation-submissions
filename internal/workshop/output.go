package workshop

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
)

// WriteCSV writes one row per operation with its timing breakdown.
func WriteCSV(w io.Writer, res *Result) error {
	cw := csv.NewWriter(w)
	header := []string{"job", "op_index", "station", "arrive", "start", "end", "leave", "queue", "process", "blocked_after"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range res.Records {
		row := []string{
			r.JobID,
			strconv.Itoa(r.OpIndex),
			r.StationID,
			strconv.Itoa(r.Arrive),
			strconv.Itoa(r.Start),
			strconv.Itoa(r.End),
			strconv.Itoa(r.Leave),
			strconv.Itoa(r.Queue),
			strconv.Itoa(r.Process),
			strconv.Itoa(r.BlockedAfter),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Summary is the JSON status document.
type Summary struct {
	Completed bool                `json:"completed"`
	Makespan  int                 `json:"makespan"`
	Stalled   bool                `json:"stalled"`
	StallTime int                 `json:"stall_time,omitempty"`
	Occupancy map[string]string   `json:"occupancy,omitempty"`
	Queues    map[string][]string `json:"queues,omitempty"`
	External  []string            `json:"external,omitempty"`
	Jobs      []JobSummary        `json:"jobs"`
}

// WriteSummary writes the status summary JSON.
func WriteSummary(w io.Writer, res *Result) error {
	s := Summary{
		Completed: res.Completed,
		Makespan:  res.Makespan,
		Stalled:   res.Stalled,
		Jobs:      res.Jobs,
	}
	if res.Stalled {
		s.StallTime = res.StallTime
		s.Occupancy = res.Occupancy
		s.Queues = res.Queues
		s.External = res.External
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
