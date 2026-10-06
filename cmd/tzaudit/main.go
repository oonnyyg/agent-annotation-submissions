// Command tzaudit audits TZif v2/v3 files: it maps absolute times to local
// wall-clock times and back, strictly within the 64-bit transition window.
//
// Usage: go run ./cmd/tzaudit query.json
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/tzif-transition-audit/tzif"
)

type request struct {
	TZif    string  `json:"tzif"`
	Queries []query `json:"queries"`
}

type query struct {
	Type  string `json:"type"`            // "at", "local", or "intervals"
	Unix  *int64 `json:"unix,omitempty"`  // for "at"
	Local string `json:"local,omitempty"` // for "local": YYYY-MM-DDTHH:MM:SS
}

type response struct {
	Window  windowInfo `json:"window"`
	Results []result   `json:"results"`
}

type windowInfo struct {
	FirstUnix int64  `json:"first_unix"`
	LastUnix  int64  `json:"last_unix"`
	FirstUTC  string `json:"first_utc"`
	LastUTC   string `json:"last_utc"`
}

type result struct {
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Error  string `json:"error,omitempty"`

	// at
	Unix     *int64 `json:"unix,omitempty"`
	UTC      string `json:"utc,omitempty"`
	Local    string `json:"local,omitempty"`
	Offset   *int64 `json:"offset_seconds,omitempty"`
	Abbr     string `json:"abbr,omitempty"`

	// local
	QueryLocal string   `json:"query_local,omitempty"`
	Matches    []match  `json:"matches,omitempty"`
	Gap        *gapInfo `json:"gap,omitempty"`

	// intervals
	Intervals []intervalInfo `json:"intervals,omitempty"`
}

type match struct {
	Unix   int64  `json:"unix"`
	UTC    string `json:"utc"`
	Offset int64  `json:"offset_seconds"`
	Abbr   string `json:"abbr"`
}

type gapInfo struct {
	StartLocal string `json:"start_local"`
	EndLocal   string `json:"end_local"`
}

type intervalInfo struct {
	StartUnix int64  `json:"start_unix"`
	EndUnix   int64  `json:"end_unix"`
	StartUTC  string `json:"start_utc"`
	EndUTC    string `json:"end_utc"`
	Offset    int64  `json:"offset_seconds"`
	Abbr      string `json:"abbr"`
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tzaudit:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: tzaudit <query.json>")
		os.Exit(2)
	}
	qdata, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	var req request
	if err := json.Unmarshal(qdata, &req); err != nil {
		fail(fmt.Errorf("parsing query file: %w", err))
	}
	tdata, err := os.ReadFile(req.TZif)
	if err != nil {
		fail(err)
	}
	f, err := tzif.Parse(tdata)
	if err != nil {
		fail(err)
	}
	first, last := f.Window()
	resp := response{
		Window: windowInfo{
			FirstUnix: first, LastUnix: last,
			FirstUTC: tzif.FormatCivil(first), LastUTC: tzif.FormatCivil(last),
		},
	}
	for i, q := range req.Queries {
		resp.Results = append(resp.Results, runQuery(f, i, q))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		fail(err)
	}
}

func runQuery(f *tzif.File, idx int, q query) (r result) {
	r.Index = idx
	r.Type = q.Type
	switch q.Type {
	case "at":
		if q.Unix == nil {
			r.Error = `query type "at" requires "unix"`
			return
		}
		r.Unix = q.Unix
		local, typ, err := f.At(*q.Unix)
		if err != nil {
			r.Error = err.Error()
			return
		}
		r.UTC = tzif.FormatCivil(*q.Unix)
		r.Local = tzif.FormatCivil(local)
		r.Offset = &typ.Offset
		r.Abbr = typ.Abbr
	case "local":
		r.QueryLocal = q.Local
		local, err := tzif.ParseCivil(q.Local)
		if err != nil {
			r.Error = err.Error()
			return
		}
		res, err := f.FromLocal(local)
		if err != nil {
			r.Error = err.Error()
			return
		}
		for _, t := range res.Times {
			_, typ, _ := f.At(t)
			r.Matches = append(r.Matches, match{
				Unix: t, UTC: tzif.FormatCivil(t), Offset: typ.Offset, Abbr: typ.Abbr,
			})
		}
		if res.Gap != nil {
			r.Gap = &gapInfo{
				StartLocal: tzif.FormatCivil(res.Gap[0]),
				EndLocal:   tzif.FormatCivil(res.Gap[1]),
			}
		}
	case "intervals":
		for _, iv := range f.Intervals() {
			r.Intervals = append(r.Intervals, intervalInfo{
				StartUnix: iv.Start, EndUnix: iv.End,
				StartUTC: tzif.FormatCivil(iv.Start), EndUTC: tzif.FormatCivil(iv.End),
				Offset: iv.Offset, Abbr: iv.Abbr,
			})
		}
	default:
		r.Error = fmt.Sprintf("unknown query type %q: want at, local, or intervals", q.Type)
	}
	return
}
