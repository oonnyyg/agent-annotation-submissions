package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func loadOrNew(path string) *Register {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return NewRegister()
	}
	r, err := Load(path)
	if err != nil {
		fatal(err)
	}
	return r
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatal(err)
	}
}

// parseJSONL reads one Revision per non-empty line.
func parseJSONL(path string) ([]Revision, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var items []Revision
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var rev Revision
		dec := json.NewDecoder(strings.NewReader(text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rev); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		items = append(items, rev)
	}
	return items, sc.Err()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: register <submit|query|timeline|diff> [flags]")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	db := fs.String("db", "register.json", "path to JSON archive")
	switch cmd {
	case "submit":
		at := fs.Int64("at", 0, "record time of the batch (integer seconds, strictly increasing)")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 1 {
			fatal(fmt.Errorf("usage: register submit -db FILE -at SECONDS batch.jsonl"))
		}
		items, err := parseJSONL(fs.Arg(0))
		if err != nil {
			fatal(err)
		}
		reg := loadOrNew(*db)
		if err := reg.Submit(Batch{RecordTime: *at, Items: items}); err != nil {
			fatal(err) // archive untouched on failure
		}
		if err := reg.Save(*db); err != nil {
			fatal(err)
		}
		fmt.Printf("applied %d revision(s) at record time %d\n", len(items), *at)
	case "query":
		device := fs.String("device", "", "device id")
		date := fs.String("date", "", "business date YYYY-MM-DD")
		asof := fs.Int64("asof", 0, "record-time cutoff (integer seconds)")
		fs.Parse(os.Args[2:])
		reg := loadOrNew(*db)
		unit, found, err := reg.OwnerAt(*device, *date, *asof)
		if err != nil {
			fatal(err)
		}
		if !found {
			printJSON(map[string]any{"device": *device, "date": *date, "unit": nil, "recorded": false})
		} else {
			printJSON(map[string]any{"device": *device, "date": *date, "unit": unit, "recorded": true})
		}
	case "timeline":
		device := fs.String("device", "", "device id")
		asof := fs.Int64("asof", 0, "record-time cutoff (integer seconds)")
		fs.Parse(os.Args[2:])
		reg := loadOrNew(*db)
		printJSON(reg.Timeline(*device, *asof))
	case "diff":
		device := fs.String("device", "", "device id")
		from := fs.Int64("from", 0, "earlier record-time cutoff")
		to := fs.Int64("to", 0, "later record-time cutoff")
		fs.Parse(os.Args[2:])
		reg := loadOrNew(*db)
		printJSON(reg.Diff(*device, *from, *to))
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		os.Exit(2)
	}
}
