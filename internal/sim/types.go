package sim

// Station is a single machine with a waiting buffer.
// Buffer capacity does not include the part being processed.
type Station struct {
	ID     string `json:"id"`
	Buffer int    `json:"buffer"`
}

// Operation is one step of a job route.
type Operation struct {
	Station  string `json:"station"`
	Duration int    `json:"duration"`
}

// Job is a work order with a release time and a finite route.
type Job struct {
	ID      string      `json:"id"`
	Release int         `json:"release"`
	Route   []Operation `json:"route"`
}

// Input is the validated problem instance.
type Input struct {
	Stations []Station `json:"stations"`
	Jobs     []Job     `json:"jobs"`
}

// OpRecord describes one executed operation for CSV output.
type OpRecord struct {
	JobID     string
	OpIndex   int
	StationID string
	Enqueue   int
	Start     int
	End       int
	Leave     int
}

// JobStats accumulates per-job durations.
type JobStats struct {
	JobID      string `json:"job_id"`
	Processing int    `json:"processing"`
	Queue      int    `json:"queue"`
	Blocked    int    `json:"blocked"`
	Completion int    `json:"completion"`
	Finished   bool   `json:"finished"`
}

// Occupancy describes a machine held by a blocked part at stall time.
type Occupancy struct {
	StationID string   `json:"station_id"`
	JobID     string   `json:"job_id"`
	Next      string   `json:"next_station"`
	Queued    []string `json:"queued,omitempty"`
}

// StallInfo reports a deadlocked state: unfinished jobs, no future
// events and no feasible transfer.
type StallInfo struct {
	Time      int         `json:"time"`
	Occupancy []Occupancy `json:"occupancy"`
	Waiting   []string    `json:"waiting"`
}

// Summary is the JSON status summary.
type Summary struct {
	Status   string     `json:"status"`
	Makespan int        `json:"makespan"`
	Jobs     []JobStats `json:"jobs"`
	Stall    *StallInfo `json:"stall,omitempty"`
}
