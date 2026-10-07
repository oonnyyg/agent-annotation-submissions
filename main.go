package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
)

func main() {
	flowsPath := flag.String("flows", "", "CSV file: flow,quantum,wait_limit_bytes")
	eventsPath := flag.String("events", "", "CSV file: time,flow,packet,length_bytes")
	rate := flag.Float64("rate", 0, "link rate in bytes per second (must be > 0)")
	flag.Parse()
	if *flowsPath == "" || *eventsPath == "" || *rate <= 0 {
		fmt.Fprintln(os.Stderr, "usage: drr-replay -flows flows.csv -events events.csv -rate BYTES_PER_SEC")
		os.Exit(2)
	}
	cfgs, err := readFlows(*flowsPath)
	if err != nil {
		fatal(err)
	}
	events, err := readEvents(*eventsPath)
	if err != nil {
		fatal(err)
	}
	sim, err := NewSimulator(*rate, cfgs)
	if err != nil {
		fatal(err)
	}
	if err := sim.Run(events); err != nil {
		fatal(err)
	}
	sends, drops, stats := sim.Results()
	printReport(sends, drops, stats)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func openCSV(path string) (*csv.Reader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	return r, func() { f.Close() }, nil
}

// readFlows parses "flow,quantum,wait_limit_bytes" rows (header required).
func readFlows(path string) ([]FlowConfig, error) {
	r, closeFn, err := openCSV(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	var cfgs []FlowConfig
	line := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		line++
		if line == 1 {
			continue // header
		}
		if len(rec) != 3 {
			return nil, fmt.Errorf("%s:%d: want 3 columns, got %d", path, line, len(rec))
		}
		quantum, err := strconv.ParseInt(rec[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: bad quantum: %w", path, line, err)
		}
		limit, err := strconv.ParseInt(rec[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: bad wait limit: %w", path, line, err)
		}
		cfgs = append(cfgs, FlowConfig{ID: rec[0], Quantum: quantum, WaitLimit: limit})
	}
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("%s: no flows configured", path)
	}
	return cfgs, nil
}

// readEvents parses "time,flow,packet,length_bytes" rows (header required).
func readEvents(path string) ([]Arrival, error) {
	r, closeFn, err := openCSV(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	var events []Arrival
	line := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		line++
		if line == 1 {
			continue // header
		}
		if len(rec) != 4 {
			return nil, fmt.Errorf("%s:%d: want 4 columns, got %d", path, line, len(rec))
		}
		t, err := strconv.ParseFloat(rec[0], 64)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: bad time: %w", path, line, err)
		}
		length, err := strconv.ParseInt(rec[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: bad length: %w", path, line, err)
		}
		events = append(events, Arrival{
			Time: t, Flow: rec[1], Packet: rec[2], Length: length, Order: len(events),
		})
	}
	return events, nil
}

func printReport(sends []SendRecord, drops []DropRecord, stats []FlowStat) {
	fmt.Println("== Send timeline ==")
	fmt.Println("start_s\tend_s\tflow\tpacket\tbytes")
	for _, s := range sends {
		fmt.Printf("%.6f\t%.6f\t%s\t%s\t%d\n", s.Start, s.End, s.Flow, s.Packet, s.Length)
	}
	fmt.Println("== Drops ==")
	if len(drops) == 0 {
		fmt.Println("(none)")
	}
	for _, d := range drops {
		fmt.Printf("%.6f\t%s\t%s\t%d\t%s\n", d.Time, d.Flow, d.Packet, d.Length, d.Reason)
	}
	fmt.Println("== Per-flow statistics ==")
	fmt.Println("flow\tarrived\tadmitted\tsent\tdropped\tadmit_bytes\tsent_bytes\tdrop_bytes\tavg_wait_s\tmax_wait_s")
	for _, st := range stats {
		avg := 0.0
		if st.Sent > 0 {
			avg = st.TotalWait / float64(st.Sent)
		}
		fmt.Printf("%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%.6f\t%.6f\n",
			st.Flow, st.Arrived, st.Admitted, st.Sent, st.Dropped,
			st.AdmitBytes, st.SentBytes, st.DropBytes, avg, st.MaxWait)
	}
}
