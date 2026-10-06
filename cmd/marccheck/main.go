// Command marccheck reads consecutive binary MARC21 records (UTF-8 only),
// writes a bibliographic CSV and a JSON diagnostics report with structural
// errors and ISBN-based record associations. It never goes online.
//
// Usage: go run ./cmd/marccheck [-csv out.csv] [-json out.json] input.mrc
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"example.com/marc-isbn-catalog/marc"
)

type diagnostics struct {
	RecordsParsed int             `json:"records_parsed"`
	BadRecords    []badRecordJSON `json:"bad_records"`
	StoppedAt     *int            `json:"stopped_at,omitempty"`
	StopReason    string          `json:"stop_reason,omitempty"`
	Groups        []groupJSON     `json:"groups"`
}

type badRecordJSON struct {
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Error  string `json:"error"`
}

type groupJSON struct {
	ID       int      `json:"id"`
	Members  []int    `json:"members"`
	ISBNs    []string `json:"isbns"`
	Conflict bool     `json:"conflict"`
}

func main() {
	csvPath := flag.String("csv", "catalog.csv", "output CSV path")
	jsonPath := flag.String("json", "diagnostics.json", "output JSON diagnostics path")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: marccheck [-csv out.csv] [-json out.json] input.mrc")
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	records, bad, stopErr := marc.ReadAll(f)

	entries := make([]marc.Entry, len(records))
	for i, rec := range records {
		entries[i] = marc.ExtractEntry(rec, i)
	}
	groups := marc.BuildGroups(entries)
	groupOf := map[int]int{}
	for _, g := range groups {
		for _, m := range g.Members {
			groupOf[m] = g.ID
		}
	}

	if err := writeCSV(*csvPath, entries, groups, groupOf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeJSON(*jsonPath, entries, bad, groups, stopErr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("parsed %d records, %d bad, %d groups\n", len(entries), len(bad), len(groups))
}

func writeCSV(path string, entries []marc.Entry, groups []marc.Group, groupOf map[int]int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	conflictGroups := map[int]bool{}
	for _, g := range groups {
		conflictGroups[g.ID] = g.Conflict
	}
	if err := w.Write([]string{"index", "control_number", "title", "isbns", "invalid_isbns", "group_id", "conflict"}); err != nil {
		return err
	}
	for _, e := range entries {
		gid := groupOf[e.Index]
		row := []string{
			fmt.Sprint(e.Index),
			e.ControlNumber,
			e.Title,
			strings.Join(e.ISBNs, " "),
			strings.Join(e.InvalidISBNs, " "),
			fmt.Sprint(gid),
			fmt.Sprint(conflictGroups[gid]),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeJSON(path string, entries []marc.Entry, bad []marc.BadRecord, groups []marc.Group, stopErr error) error {
	d := diagnostics{RecordsParsed: len(entries), Groups: []groupJSON{}, BadRecords: []badRecordJSON{}}
	for _, b := range bad {
		d.BadRecords = append(d.BadRecords, badRecordJSON{Offset: b.Offset, Length: b.Length, Error: b.Err})
	}
	if stopErr != nil {
		if pe, ok := stopErr.(*marc.ParseError); ok {
			off := pe.Offset
			d.StoppedAt = &off
		}
		d.StopReason = stopErr.Error()
	}
	for _, g := range groups {
		d.Groups = append(d.Groups, groupJSON{ID: g.ID, Members: g.Members, ISBNs: g.ISBNs, Conflict: g.Conflict})
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
