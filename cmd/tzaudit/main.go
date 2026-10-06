// Command tzaudit audits a local TZif file and answers time-conversion
// queries described by a JSON input file. It never reads the OS time zone
// database or the network.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"example.com/tzif-transition-audit/tzif"
)

type input struct {
	ZoneFile string  `json:"zone_file"`
	Queries  []query `json:"queries"`
}

type query struct {
	Type  string `json:"type"`            // "at", "local", or "intervals"
	Unix  *int64 `json:"unix,omitempty"`  // for "at"
	Local string `json:"local,omitempty"` // for "local": "2006-01-02T15:04:05"
}

type result struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	// at
	Unix   *int64 `json:"unix,omitempty"`
	Local  string `json:"local,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Abbr   string `json:"abbr,omitempty"`

	// local
	Matches []matchOut `json:"matches,omitempty"`
	Gap     *gapOut    `json:"gap,omitempty"`

	// intervals
	Intervals []intervalOut `json:"intervals,omitempty"`
}

type matchOut struct {
	Unix   int64  `json:"unix"`
	Offset int64  `json:"offset"`
	Abbr   string `json:"abbr"`
}

type gapOut struct {
	StartLocal string `json:"start_local"`
	EndLocal   string `json:"end_local"`
	AtUnix     int64  `json:"at_unix"`
}

type intervalOut struct {
	StartUnix int64  `json:"start_unix"`
	EndUnix   int64  `json:"end_unix"`
	Offset    int64  `json:"offset"`
	Abbr      string `json:"abbr"`
	IsDST     bool   `json:"is_dst"`
}

type output struct {
	ZoneFile string   `json:"zone_file"`
	Window   window   `json:"window"`
	Results  []result `json:"results"`
}

type window struct {
	FirstUnix int64 `json:"first_unix"`
	LastUnix  int64 `json:"last_unix"`
}

const localLayout = "2006-01-02T15:04:05"

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "tzaudit: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	if len(os.Args) != 2 {
		fail("usage: tzaudit <query.json>")
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail("reading query file: %v", err)
	}
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		fail("parsing query file: %v", err)
	}
	raw, err := os.ReadFile(in.ZoneFile)
	if err != nil {
		fail("reading zone file: %v", err)
	}
	zone, err := tzif.Parse(raw)
	if err != nil {
		fail("%v", err)
	}
	first, last := zone.Window()
	out := output{ZoneFile: in.ZoneFile, Window: window{FirstUnix: first, LastUnix: last}}
	for _, q := range in.Queries {
		out.Results = append(out.Results, runQuery(zone, q))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fail("encoding output: %v", err)
	}
}

func runQuery(z *tzif.Zone, q query) (r result) {
	switch q.Type {
	case "at":
		if q.Unix == nil {
			r.Error = `query "at" requires "unix"`
			return r
		}
		r.Unix = q.Unix
		wall, lt, err := z.At(*q.Unix)
		if err != nil {
			r.Error = err.Error()
			return r
		}
		r.OK = true
		r.Local = wall.Format(localLayout)
		r.Offset = lt.Offset
		r.Abbr = lt.Abbr
	case "local":
		t, err := time.ParseInLocation(localLayout, q.Local, time.UTC)
		if err != nil {
			r.Error = fmt.Sprintf("bad local time %q: %v", q.Local, err)
			return r
		}
		matches, gap := z.LookupLocal(t)
		r.OK = true
		r.Matches = []matchOut{}
		for _, m := range matches {
			r.Matches = append(r.Matches, matchOut{Unix: m.Unix, Offset: m.Offset, Abbr: m.Abbr})
		}
		if gap != nil {
			r.Gap = &gapOut{
				StartLocal: gap.StartLocal.Format(localLayout),
				EndLocal:   gap.EndLocal.Format(localLayout),
				AtUnix:     gap.AtUnix,
			}
		}
	case "intervals":
		r.OK = true
		r.Intervals = []intervalOut{}
		for _, iv := range z.Intervals() {
			r.Intervals = append(r.Intervals, intervalOut{
				StartUnix: iv.StartUnix,
				EndUnix:   iv.EndUnix,
				Offset:    iv.Offset,
				Abbr:      iv.Abbr,
				IsDST:     iv.IsDST,
			})
		}
	default:
		r.Error = fmt.Sprintf("unknown query type %q", q.Type)
	}
	return r
}