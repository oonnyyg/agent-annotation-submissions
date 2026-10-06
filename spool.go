package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Sheet describes one physical sheet of paper in a job plan.
type Sheet struct {
	Copy  int  `json:"copy"`
	Front *int `json:"front"` // page number on the front side, nil if blank
	Back  *int `json:"back"`  // page number on the back side, nil if blank
}

func (s Sheet) String() string {
	page := func(p *int) string {
		if p == nil {
			return "blank"
		}
		return fmt.Sprintf("p%d", *p)
	}
	return fmt.Sprintf("copy%d front=%s back=%s", s.Copy, page(s.Front), page(s.Back))
}

const (
	StatusPending   = "pending"
	StatusPaused    = "paused"
	StatusPrinting  = "printing"
	StatusDone      = "done"
	StatusCancelled = "cancelled"
)

// Job is a queued print job with its per-sheet plan and progress.
type Job struct {
	ID        string  `json:"id"`
	User      string  `json:"user"`
	Doc       string  `json:"doc"`
	Pages     []int   `json:"pages"` // selected source pages, sorted, deduplicated
	Copies    int     `json:"copies"`
	Duplex    bool    `json:"duplex"`
	Plan      []Sheet `json:"plan"`
	Confirmed int     `json:"confirmed"` // number of leading sheets confirmed printed
	Status    string  `json:"status"`
	Seq       int     `json:"seq"` // submission order
}

// User tracks quota and paper accounting.
type User struct {
	Name     string `json:"name"`
	Quota    int    `json:"quota"`    // total allowance in sheets
	Reserved int    `json:"reserved"` // sheets reserved by unfinished jobs
	Used     int    `json:"used"`     // sheets actually consumed
}

// Spool is the single-printer job queue with user quotas.
type Spool struct {
	Users  map[string]*User `json:"users"`
	Jobs   []*Job           `json:"jobs"`
	NextID int              `json:"next_id"`
}

func NewSpool() *Spool {
	return &Spool{Users: map[string]*User{}, NextID: 1}
}

func (s *Spool) RegisterUser(name string, quota int) error {
	if name == "" || quota < 0 {
		return errors.New("invalid user or quota")
	}
	if _, ok := s.Users[name]; ok {
		return fmt.Errorf("user %q already registered", name)
	}
	s.Users[name] = &User{Name: name, Quota: quota}
	return nil
}

// ParseRanges parses ranges like "1-3,5,8-7" into sorted unique page numbers.
// Any invalid or out-of-bounds range rejects the whole request.
func ParseRanges(spec string, totalPages int) ([]int, error) {
	set := map[int]bool{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty range in %q", spec)
		}
		lo, hi := 0, 0
		if strings.Contains(part, "-") {
			var a, b int
			if _, err := fmt.Sscanf(part, "%d-%d", &a, &b); err != nil {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			lo, hi = a, b
			if lo > hi {
				lo, hi = hi, lo
			}
		} else {
			if _, err := fmt.Sscanf(part, "%d", &lo); err != nil {
				return nil, fmt.Errorf("invalid page %q", part)
			}
			hi = lo
		}
		if lo < 1 || hi > totalPages {
			return nil, fmt.Errorf("range %q out of bounds 1..%d", part, totalPages)
		}
		for p := lo; p <= hi; p++ {
			set[p] = true
		}
	}
	if len(set) == 0 {
		return nil, errors.New("no pages selected")
	}
	pages := make([]int, 0, len(set))
	for p := range set {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	return pages, nil
}

// BuildPlan arranges selected pages into sheets per copy. Every copy starts
// on a front side; a trailing odd page leaves its back side blank.
func BuildPlan(pages []int, copies int, duplex bool) []Sheet {
	var plan []Sheet
	for c := 1; c <= copies; c++ {
		if !duplex {
			for _, p := range pages {
				pg := p
				plan = append(plan, Sheet{Copy: c, Front: &pg})
			}
			continue
		}
		for i := 0; i < len(pages); i += 2 {
			front := pages[i]
			sh := Sheet{Copy: c, Front: &front}
			if i+1 < len(pages) {
				back := pages[i+1]
				sh.Back = &back
			}
			plan = append(plan, sh)
		}
	}
	return plan
}

// Submit validates and enqueues a job, reserving its full sheet count.
func (s *Spool) Submit(user, doc string, totalPages int, ranges string, copies int, duplex bool) (*Job, error) {
	u, ok := s.Users[user]
	if !ok {
		return nil, fmt.Errorf("unknown user %q", user)
	}
	if totalPages < 1 || copies < 1 {
		return nil, errors.New("invalid page count or copies")
	}
	pages, err := ParseRanges(ranges, totalPages)
	if err != nil {
		return nil, err // invalid range rejects the whole submission
	}
	plan := BuildPlan(pages, copies, duplex)
	if u.Used+u.Reserved+len(plan) > u.Quota {
		return nil, fmt.Errorf("quota exceeded: need %d sheets, available %d", len(plan), u.Quota-u.Used-u.Reserved)
	}
	job := &Job{
		ID:     fmt.Sprintf("job-%d", s.NextID),
		User:   user,
		Doc:    doc,
		Pages:  pages,
		Copies: copies,
		Duplex: duplex,
		Plan:   plan,
		Status: StatusPending,
		Seq:    s.NextID,
	}
	s.NextID++
	s.Jobs = append(s.Jobs, job)
	u.Reserved += len(plan)
	return job, nil
}

func (s *Spool) findJob(id string) (*Job, error) {
	for _, j := range s.Jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return nil, fmt.Errorf("unknown job %q", id)
}

// activeJob returns the first unfinished, non-paused job in submission order.
func (s *Spool) activeJob() *Job {
	for _, j := range s.Jobs {
		if j.Status == StatusPending || j.Status == StatusPrinting {
			return j
		}
	}
	return nil
}

// Pause suspends a job that has not started printing.
func (s *Spool) Pause(id string) error {
	j, err := s.findJob(id)
	if err != nil {
		return err
	}
	if j.Status != StatusPending {
		return fmt.Errorf("only a job that has not started can be paused (status %s)", j.Status)
	}
	j.Status = StatusPaused
	return nil
}

// Resume re-enqueues a paused job at its original submission position.
func (s *Spool) Resume(id string) error {
	j, err := s.findJob(id)
	if err != nil {
		return err
	}
	if j.Status != StatusPaused {
		return fmt.Errorf("job %s is not paused", id)
	}
	j.Status = StatusPending
	return nil
}

// Confirm marks the next unconfirmed sheet of the active job as printed,
// converting one reserved sheet into actual consumption. Confirming an
// already-printed sheet or skipping ahead is rejected.
func (s *Spool) Confirm(id string, sheet int) error {
	j, err := s.findJob(id)
	if err != nil {
		return err
	}
	if j.Status == StatusDone || j.Status == StatusCancelled {
		return fmt.Errorf("job %s already finished (%s)", id, j.Status)
	}
	if j.Status == StatusPaused {
		return fmt.Errorf("job %s is paused", id)
	}
	if active := s.activeJob(); active != j {
		return fmt.Errorf("job %s is not the active job", id)
	}
	next := j.Confirmed + 1
	if sheet < next {
		return fmt.Errorf("sheet %d already confirmed", sheet)
	}
	if sheet > next {
		return fmt.Errorf("cannot skip unconfirmed sheet %d", next)
	}
	j.Confirmed++
	j.Status = StatusPrinting
	u := s.Users[j.User]
	u.Reserved--
	u.Used++
	if j.Confirmed == len(j.Plan) {
		j.Status = StatusDone
	}
	return nil
}

// Cancel releases the reservation for sheets not yet printed. Consumed
// sheets and history are kept.
func (s *Spool) Cancel(id string) error {
	j, err := s.findJob(id)
	if err != nil {
		return err
	}
	if j.Status == StatusDone || j.Status == StatusCancelled {
		return fmt.Errorf("job %s already finished (%s)", id, j.Status)
	}
	remaining := len(j.Plan) - j.Confirmed
	u := s.Users[j.User]
	u.Reserved -= remaining
	j.Status = StatusCancelled
	return nil
}

// Report returns the paper usage summary for a user.
func (s *Spool) Report(name string) (quota, reserved, used, remaining int, err error) {
	u, ok := s.Users[name]
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("unknown user %q", name)
	}
	return u.Quota, u.Reserved, u.Used, u.Quota - u.Reserved - u.Used, nil
}
