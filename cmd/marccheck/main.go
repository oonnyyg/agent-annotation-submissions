// Command marccheck reads consecutive MARC21 binary records (UTF-8 only)
// and writes a bibliographic CSV plus structural/linkage diagnostics JSON.
//
// Usage:
//
//	go run ./cmd/marccheck [-csv out.csv] [-json out.json] <input.mrc>
//
// Use "-" for stdout. Defaults: catalog.csv and diagnostics.json.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"example.com/marc-isbn-catalog/internal/catalog"
	"example.com/marc-isbn-catalog/internal/marc"
)

type diagnostics struct {
	Structural []structuralDiag `json:"structural"`
	Groups     []catalog.Group  `json:"groups"`
	Summary    summary          `json:"summary"`
}

type structuralDiag struct {
	Offset  int    `json:"offset"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

type summary struct {
	RecordsParsed  int  `json:"records_parsed"`
	RecordsBad     int  `json:"records_bad"`
	Groups         int  `json:"groups"`
	ConflictGroups int  `json:"conflict_groups"`
	StoppedEarly   bool `json:"stopped_early"`
}

func main() {
	csvPath := flag.String("csv", "catalog.csv", "CSV output path, or - for stdout")
	jsonPath := flag.String("json", "diagnostics.json", "JSON diagnostics path, or - for stdout")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: marccheck [-csv path|-] [-json path|-] <input.mrc>")
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read input:", err)
		os.Exit(1)
	}

	records, perrs := marc.Parse(data)
	entries := catalog.ExtractAll(records)
	groups := catalog.Groups(entries)

	if err := writeCSV(*csvPath, entries); err != nil {
		fmt.Fprintln(os.Stderr, "write csv:", err)
		os.Exit(1)
	}
	if err := writeJSON(*jsonPath, buildDiags(perrs, groups, entries)); err != nil {
		fmt.Fprintln(os.Stderr, "write json:", err)
		os.Exit(1)
	}
}

func buildDiags(perrs []*marc.ParseError, groups []catalog.Group, entries []catalog.Entry) diagnostics {
	d := diagnostics{Groups: groups}
	for _, p := range perrs {
		d.Structural = append(d.Structural, structuralDiag{Offset: p.Offset, Message: p.Message, Fatal: p.Fatal})
	}
	if d.Structural == nil {
		d.Structural = []structuralDiag{}
	}
	d.Summary.RecordsParsed = len(entries)
	d.Summary.RecordsBad = len(perrs)
	d.Summary.Groups = len(groups)
	for _, g := range groups {
		if g.Conflict {
			d.Summary.ConflictGroups++
		}
	}
	for _, p := range perrs {
		if p.Fatal {
			d.Summary.StoppedEarly = true
		}
	}
	return d
}

func writeCSV(path string, entries []catalog.Entry) error {
	w, closeFn, err := openOut(path)
	if err != nil {
		return err
	}
	defer closeFn()
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{"index", "control_number", "title", "valid_isbns", "invalid_isbns"}); err != nil {
		return err
	}
	for _, e := range entries {
		row := []string{
			fmt.Sprint(e.Index),
			e.ControlNumber,
			e.Title,
			strings.Join(e.ValidISBNs, " "),
			strings.Join(e.InvalidISBNs, " "),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}

func writeJSON(path string, d diagnostics) error {
	w, closeFn, err := openOut(path)
	if err != nil {
		return err
	}
	defer closeFn()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

func openOut(path string) (io.Writer, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}
