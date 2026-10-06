package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type jsonReport struct {
	Start             int64               `json:"start"`
	End               int64               `json:"end"`
	TotalRequests     uint64              `json:"total_requests"`
	AvgLatency        *float64            `json:"avg_latency"` // null when zero requests
	RequestsPerSecond float64             `json:"requests_per_second"`
	Quantiles         map[string]*float64 `json:"quantiles"`
	RestartGaps       []RestartGap        `json:"restart_gaps"`
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("histwin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "path to JSONL snapshot file (required)")
	start := fs.Int64("start", 0, "window start, integer seconds (required)")
	end := fs.Int64("end", 0, "window end, integer seconds (required)")
	quantiles := fs.String("quantiles", "0.5,0.9,0.99", "comma-separated quantiles in [0,1]")
	csvPath := fs.String("csv", "", "optional path for per-bucket CSV output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *input == "" || *end <= *start {
		fmt.Fprintln(stderr, "usage: histwin -input FILE -start N -end M [-quantiles 0.5,0.9] [-csv FILE]")
		return 2
	}
	qs, err := parseQuantiles(*quantiles)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	defer f.Close()
	byInstance, err := LoadSnapshots(f)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	rep, err := MergeWindow(byInstance, *start, *end)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	out := jsonReport{
		Start:             rep.Start,
		End:               rep.End,
		TotalRequests:     rep.TotalCount,
		RequestsPerSecond: rep.RequestsPerSecond(),
		Quantiles:         map[string]*float64{},
		RestartGaps:       rep.RestartGaps,
	}
	if out.RestartGaps == nil {
		out.RestartGaps = []RestartGap{}
	}
	if avg, ok := rep.AvgLatency(); ok {
		out.AvgLatency = &avg
	}
	for _, q := range qs {
		key := strconv.FormatFloat(q, 'g', -1, 64)
		if v, ok := rep.Quantile(q); ok {
			out.Quantiles[key] = &v
		} else {
			out.Quantiles[key] = nil
		}
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if *csvPath != "" {
		if err := writeCSV(*csvPath, rep); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
	}
	return 0
}

func parseQuantiles(s string) ([]float64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var qs []float64
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || v < 0 || v > 1 {
			return nil, fmt.Errorf("invalid quantile %q: must be in [0,1]", part)
		}
		qs = append(qs, v)
	}
	return qs, nil
}

func writeCSV(path string, rep *Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"bucket_lower", "bucket_upper", "count"}); err != nil {
		return err
	}
	for i, c := range rep.BucketSegments() {
		lower := "0"
		if i > 0 {
			lower = strconv.FormatFloat(rep.Bounds[i-1], 'g', -1, 64)
		}
		upper := "+Inf"
		if i < len(rep.Bounds) {
			upper = strconv.FormatFloat(rep.Bounds[i], 'g', -1, 64)
		}
		if err := w.Write([]string{lower, upper, strconv.FormatUint(c, 10)}); err != nil {
			return err
		}
	}
	return w.Error()
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
