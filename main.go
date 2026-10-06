
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func usage() {
	fmt.Fprintln(os.Stderr, `bitemporal asset register

usage:
  register submit <archive> <batches.jsonl>   apply JSONL batches (all-or-nothing per line)
  register query  <archive> <device> <date> <cutoff>   owner at business date, as known at cutoff
  register timeline <archive> <device> <cutoff>        full business timeline as known at cutoff
  register diff   <archive> <device> <cutoffA> <cutoffB>

JSONL batch: {"recorded_at":<int seconds>,"revisions":[{"device":"D","unit":"U","from":"YYYY-MM-DD","to":"YYYY-MM-DD"}]}
empty unit revokes ownership for the span; dates are [from, to).`)
	os.Exit(2)
}

func loadOrNew(path string) (*Register, []*Batch, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return NewRegister(), nil, nil
	}
	return LoadArchive(path)
}

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	cmd, archivePath := os.Args[1], os.Args[2]
	var err error
	switch cmd {
	case "submit":
		err = cmdSubmit(archivePath, os.Args[3:])
	case "query":
		err = cmdQuery(archivePath, os.Args[3:])
	case "timeline":
		err = cmdTimeline(archivePath, os.Args[3:])
	case "diff":
		err = cmdDiff(archivePath, os.Args[3:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdSubmit(path string, args []string) error {
	if len(args) != 1 {
		usage()
	}
	reg, batches, err := loadOrNew(path)
	if err != nil {
		return err
	}
	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer f.Close()
	// Validate every line against the evolving register first; only if the
	// whole file succeeds do we persist, so a failed file never changes the
	// archive on disk.
	pending := []*Batch{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var b Batch
		dec := json.NewDecoder(strings.NewReader(text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&b); err != nil {
			return fmt.Errorf("line %d: %v", line, err)
		}
		if err := reg.ValidateBatch(&b); err != nil {
			return fmt.Errorf("line %d: %v", line, err)
		}
		pending = append(pending, &b)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for _, b := range pending {
		if err := reg.Apply(b); err != nil { // cannot happen; validated above
			return err
		}
	}
	batches = append(batches, pending...)
	if err := SaveArchive(path, batches); err != nil {
		return err
	}
	fmt.Printf("applied %d batch(es)\n", len(pending))
	return nil
}

func parseCutoff(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cutoff %q (want integer seconds)", s)
	}
	return v, nil
}

func cmdQuery(path string, args []string) error {
	if len(args) != 3 {
		usage()
	}
	reg, _, err := LoadArchive(path)
	if err != nil {
		return err
	}
	device := args[0]
	day, err := ParseDate(args[1])
	if err != nil {
		return err
	}
	cutoff, err := parseCutoff(args[2])
	if err != nil {
		return err
	}
	for _, s := range reg.Timeline(device, cutoff) {
		if !day.Before(s.From) && day.Before(s.To) {
			fmt.Printf("%s\n", s.Unit)
			return nil
		}
	}
	fmt.Println("NO RECORD")
	return nil
}

func cmdTimeline(path string, args []string) error {
	if len(args) != 2 {
		usage()
	}
	reg, _, err := LoadArchive(path)
	if err != nil {
		return err
	}
	cutoff, err := parseCutoff(args[1])
	if err != nil {
		return err
	}
	tl := reg.Timeline(args[0], cutoff)
	if len(tl) == 0 {
		fmt.Println("(no recorded ownership)")
		return nil
	}
	out, _ := json.Marshal(timelineJSON(tl))
	fmt.Println(string(out))
	return nil
}

type segmentJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
	Unit string `json:"unit"`
}

func timelineJSON(tl []Segment) []segmentJSON {
	out := make([]segmentJSON, 0, len(tl))
	for _, s := range tl {
		out = append(out, segmentJSON{s.From.String(), s.To.String(), s.Unit})
	}
	return out
}

func cmdDiff(path string, args []string) error {
	if len(args) != 3 {
		usage()
	}
	reg, _, err := LoadArchive(path)
	if err != nil {
		return err
	}
	a, err := parseCutoff(args[1])
	if err != nil {
		return err
	}
	b, err := parseCutoff(args[2])
	if err != nil {
		return err
	}
	changes := reg.Diff(args[0], a, b)
	if len(changes) == 0 {
		fmt.Println("(no changes)")
		return nil
	}
	type changeJSON struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Before string `json:"before"`
		After  string `json:"after"`
	}
	out := make([]changeJSON, 0, len(changes))
	for _, c := range changes {
		out = append(out, changeJSON{c.From.String(), c.To.String(), c.Before, c.After})
	}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
	return nil
}

var _ io.Reader = (*os.File)(nil)
