// Command workshop replays a job-shop JSON instance and explains why
// work orders waited or blocked machines.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"example.com/blocking-workshop-replay/internal/sim"
)

func main() {
	stationsPath := flag.String("stations", "stations.json", "stations JSON file")
	jobsPath := flag.String("jobs", "jobs.json", "jobs JSON file")
	csvPath := flag.String("csv", "ops.csv", "operation times CSV output")
	summaryPath := flag.String("summary", "summary.json", "status summary JSON output")
	flag.Parse()

	if err := run(*stationsPath, *jobsPath, *csvPath, *summaryPath); err != nil {
		fmt.Fprintln(os.Stderr, "workshop:", err)
		os.Exit(1)
	}
}

func run(stationsPath, jobsPath, csvPath, summaryPath string) error {
	in, err := sim.LoadInput(stationsPath, jobsPath)
	if err != nil {
		return err
	}
	s := sim.New(in)
	sum := s.Run()
	if err := writeCSV(csvPath, s.Records()); err != nil {
		return err
	}
	return writeSummary(summaryPath, sum)
}

func writeCSV(path string, records []sim.OpRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{"job", "op", "station", "enqueue", "start", "end", "leave", "queue", "blocked"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range records {
		row := []string{
			r.JobID,
			strconv.Itoa(r.OpIndex),
			r.StationID,
			strconv.Itoa(r.Enqueue),
			strconv.Itoa(r.Start),
			strconv.Itoa(r.End),
			strconv.Itoa(r.Leave),
			strconv.Itoa(r.Start - r.Enqueue),
			strconv.Itoa(r.Leave - r.End),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeSummary(path string, sum *sim.Summary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(sum)
}
