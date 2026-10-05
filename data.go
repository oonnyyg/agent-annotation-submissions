package main

import (
	"encoding/csv"
	"fmt"
	"os"
)

// Record is one input row: an identifier, quasi-identifier values and one
// sensitive value.
type Record struct {
	ID        string
	QI        []string
	Sensitive string
	Line      int // 1-based data line number in the input file
}

// Dataset holds the parsed CSV input.
type Dataset struct {
	QIColumns []string
	Records   []Record
}

// LoadDataset reads the CSV file. Layout: first column is the record id,
// the last column is the sensitive attribute, the 1..4 middle columns are
// quasi-identifiers.
func LoadDataset(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open data file: %w", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("data file is empty")
	}
	header := rows[0]
	if len(header) < 3 {
		return nil, fmt.Errorf("CSV needs at least id, one quasi-identifier and a sensitive column")
	}
	qiCols := header[1 : len(header)-1]
	if len(qiCols) > 4 {
		return nil, fmt.Errorf("at most 4 quasi-identifier columns allowed, got %d", len(qiCols))
	}
	for _, c := range header {
		if c == "" {
			return nil, fmt.Errorf("CSV header contains an empty column name")
		}
	}
	seen := map[string]bool{}
	ds := &Dataset{QIColumns: qiCols}
	for i, row := range rows[1:] {
		line := i + 1
		if len(row) != len(header) {
			return nil, fmt.Errorf("line %d: expected %d fields, got %d", line+1, len(header), len(row))
		}
		rec := Record{ID: row[0], Sensitive: row[len(row)-1], Line: line}
		if rec.ID == "" {
			return nil, fmt.Errorf("line %d: record id must not be empty", line+1)
		}
		if seen[rec.ID] {
			return nil, fmt.Errorf("line %d: duplicate record id %q", line+1, rec.ID)
		}
		seen[rec.ID] = true
		if rec.Sensitive == "" {
			return nil, fmt.Errorf("line %d: sensitive value must not be empty", line+1)
		}
		rec.QI = make([]string, len(qiCols))
		for j := range qiCols {
			if row[j+1] == "" {
				return nil, fmt.Errorf("line %d: missing value in quasi-identifier column %q", line+1, qiCols[j])
			}
			rec.QI[j] = row[j+1]
		}
		ds.Records = append(ds.Records, rec)
	}
	if len(ds.Records) == 0 {
		return nil, fmt.Errorf("data file has no records")
	}
	return ds, nil
}

// ValidateLeaves checks every quasi-identifier value against its hierarchy.
func ValidateLeaves(ds *Dataset, hs map[string]*Hierarchy) error {
	for _, col := range ds.QIColumns {
		if _, ok := hs[col]; !ok {
			return fmt.Errorf("no hierarchy provided for quasi-identifier column %q", col)
		}
	}
	for _, rec := range ds.Records {
		for j, col := range ds.QIColumns {
			if !hs[col].leaves[rec.QI[j]] {
				return fmt.Errorf("record %q: value %q is not a leaf of hierarchy %q", rec.ID, rec.QI[j], col)
			}
		}
	}
	return nil
}
