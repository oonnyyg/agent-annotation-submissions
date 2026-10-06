// Command regionaudit reads a JSON document describing polygonal regions
// and query points, validates every region, and writes a JSON report with
// per-region geometry checks, measurements, and point classifications.
//
// Usage: go run ./cmd/regionaudit [input.json]
// With no argument, input is read from standard input.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"example.com/polygon-region-audit/region"
)

type inputRegion struct {
	ID    string        `json:"id"`
	Rings []region.Ring `json:"rings"`
}

type inputDoc struct {
	Regions []inputRegion  `json:"regions"`
	Queries []region.Point `json:"queries"`
}

type regionReport struct {
	ID     string         `json:"id"`
	Valid  bool           `json:"valid"`
	Issues []region.Issue `json:"issues,omitempty"`
	Area   float64        `json:"area,omitempty"`
	// Centroid is [x, y]; omitted for invalid regions.
	Centroid  *[2]float64 `json:"centroid,omitempty"`
	Perimeter float64     `json:"perimeter,omitempty"`
}

type queryHit struct {
	RegionID string          `json:"region"`
	Location region.Location `json:"location"`
}

type queryReport struct {
	Point   region.Point `json:"point"`
	Results []queryHit   `json:"results"`
}

type outputDoc struct {
	Regions []regionReport `json:"regions"`
	Queries []queryReport  `json:"queries"`
}

func run(r io.Reader, w io.Writer) error {
	var in inputDoc
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return fmt.Errorf("decoding input: %w", err)
	}
	out := outputDoc{
		Regions: make([]regionReport, 0, len(in.Regions)),
		Queries: make([]queryReport, 0, len(in.Queries)),
	}
	valid := make([]bool, len(in.Regions))
	for i, reg := range in.Regions {
		rep := regionReport{ID: reg.ID}
		issues := region.ValidateRegion(reg.Rings)
		if len(issues) == 0 {
			rep.Valid = true
			m := region.Measure(reg.Rings)
			rep.Area = m.Area
			rep.Centroid = &m.Centroid
			rep.Perimeter = m.Perimeter
		} else {
			rep.Issues = issues
		}
		valid[i] = rep.Valid
		out.Regions = append(out.Regions, rep)
	}
	for _, q := range in.Queries {
		qr := queryReport{Point: q, Results: make([]queryHit, 0, len(in.Regions))}
		for i, reg := range in.Regions {
			if !valid[i] {
				continue // invalid regions do not participate in point location
			}
			qr.Results = append(qr.Results, queryHit{
				RegionID: reg.ID,
				Location: region.Locate(q, reg.Rings),
			})
		}
		out.Queries = append(out.Queries, qr)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func main() {
	var src io.Reader = os.Stdin
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: regionaudit [input.json]")
		os.Exit(2)
	}
	if len(os.Args) == 2 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		src = f
	}
	if err := run(src, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
