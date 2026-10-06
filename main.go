package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// Command is one JSONL request line.
type Command struct {
	Cmd        string `json:"cmd"`
	User       string `json:"user,omitempty"`
	Quota      int    `json:"quota,omitempty"`
	Doc        string `json:"doc,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
	Ranges     string `json:"ranges,omitempty"`
	Copies     int    `json:"copies,omitempty"`
	Duplex     bool   `json:"duplex,omitempty"`
	Job        string `json:"job,omitempty"`
	Sheet      int    `json:"sheet,omitempty"`
}

func reply(ok bool, data map[string]any, err error) map[string]any {
	r := map[string]any{"ok": ok}
	if err != nil {
		r["error"] = err.Error()
	}
	for k, v := range data {
		r[k] = v
	}
	return r
}

func execute(s *Spool, c Command) map[string]any {
	switch c.Cmd {
	case "register_user":
		err := s.RegisterUser(c.User, c.Quota)
		return reply(err == nil, nil, err)
	case "submit":
		job, err := s.Submit(c.User, c.Doc, c.TotalPages, c.Ranges, c.Copies, c.Duplex)
		if err != nil {
			return reply(false, nil, err)
		}
		return reply(true, map[string]any{"job": job.ID, "sheets": len(job.Plan), "pages": job.Pages}, nil)
	case "pause":
		err := s.Pause(c.Job)
		return reply(err == nil, nil, err)
	case "resume":
		err := s.Resume(c.Job)
		return reply(err == nil, nil, err)
	case "confirm":
		err := s.Confirm(c.Job, c.Sheet)
		return reply(err == nil, nil, err)
	case "cancel":
		err := s.Cancel(c.Job)
		return reply(err == nil, nil, err)
	case "plan":
		j, err := s.findJob(c.Job)
		if err != nil {
			return reply(false, nil, err)
		}
		sheets := make([]map[string]any, 0, len(j.Plan))
		for i, sh := range j.Plan {
			sheets = append(sheets, map[string]any{
				"sheet": i + 1, "copy": sh.Copy,
				"front": sh.Front, "back": sh.Back,
				"printed": i < j.Confirmed,
			})
		}
		return reply(true, map[string]any{"job": j.ID, "status": j.Status, "sheets": sheets}, nil)
	case "status":
		jobs := make([]map[string]any, 0, len(s.Jobs))
		for _, j := range s.Jobs {
			jobs = append(jobs, map[string]any{
				"job": j.ID, "user": j.User, "doc": j.Doc, "status": j.Status,
				"progress": fmt.Sprintf("%d/%d", j.Confirmed, len(j.Plan)),
			})
		}
		active := ""
		if a := s.activeJob(); a != nil {
			active = a.ID
		}
		return reply(true, map[string]any{"active": active, "jobs": jobs}, nil)
	case "report":
		quota, reserved, used, remaining, err := s.Report(c.User)
		if err != nil {
			return reply(false, nil, err)
		}
		return reply(true, map[string]any{
			"user": c.User, "quota": quota, "reserved": reserved,
			"used": used, "remaining": remaining,
		}, nil)
	default:
		return reply(false, nil, fmt.Errorf("unknown command %q", c.Cmd))
	}
}

func load(path string) *Spool {
	data, err := os.ReadFile(path)
	if err != nil {
		return NewSpool()
	}
	s := NewSpool()
	if err := json.Unmarshal(data, s); err != nil {
		fmt.Fprintf(os.Stderr, "warning: corrupt state file, starting fresh: %v\n", err)
		return NewSpool()
	}
	if s.Users == nil {
		s.Users = map[string]*User{}
	}
	return s
}

func save(path string, s *Spool) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func main() {
	statePath := flag.String("state", "spool_state.json", "path to persistent state file")
	flag.Parse()

	spool := load(*statePath)
	out := json.NewEncoder(os.Stdout)
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var cmd Command
		var res map[string]any
		if err := json.Unmarshal(line, &cmd); err != nil {
			res = reply(false, nil, fmt.Errorf("invalid JSON: %v", err))
		} else {
			res = execute(spool, cmd)
			if err := save(*statePath, spool); err != nil {
				fmt.Fprintf(os.Stderr, "warning: save failed: %v\n", err)
			}
		}
		if err := out.Encode(res); err != nil {
			fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
			return
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read failed: %v\n", err)
	}
}
