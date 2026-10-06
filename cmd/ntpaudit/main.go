// Command ntpaudit audits recorded NTP exchanges from a JSONL file and
// reports per-record analysis plus per-peer offset summaries. It is
// strictly offline: it never sends packets or sets the system clock.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"example.com/ntp-exchange-auditor/internal/audit"
)

func main() {
	window := flag.Int("window", 8, "number of recent valid samples kept per peer")
	flag.Parse()

	var in io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "ntpaudit:", err)
			os.Exit(2)
		}
		defer f.Close()
		in = f
	}
	if *window < 1 {
		fmt.Fprintln(os.Stderr, "ntpaudit: -window must be >= 1")
		os.Exit(2)
	}
	if err := run(in, os.Stdout, *window); err != nil {
		fmt.Fprintln(os.Stderr, "ntpaudit:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer, window int) error {
	peers := map[string]*audit.Peer{}
	var order []string
	getPeer := func(name string) *audit.Peer {
		p, ok := peers[name]
		if !ok {
			p = audit.NewPeer(name, window)
			peers[name] = p
			order = append(order, name)
		}
		return p
	}

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var rec audit.Record
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			fmt.Fprintf(out, "line %d: REJECT bad-json: %v\n", line, err)
			continue
		}
		res := audit.Analyze(line, rec)
		peer := getPeer(rec.Peer)
		if res.OK {
			if peer.Add(rec, res) {
				fmt.Fprintf(out, "line %d peer %q: DUPLICATE response for an already-answered request; ignored\n", line, rec.Peer)
				continue
			}
			s := res.Sample
			fmt.Fprintf(out, "line %d peer %q: VALID offset=%v delay=%v\n", line, rec.Peer, s.Offset, s.Delay)
		} else {
			peer.Add(rec, res) // tracks duplicates even for rejected records
			fmt.Fprintf(out, "line %d peer %q: REJECT %s: %s\n", line, rec.Peer, res.Kind, res.Detail)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	sort.Strings(order)
	fmt.Fprintln(out, "\n== peer summaries ==")
	for _, name := range order {
		p := peers[name]
		best, ok := p.Best()
		if !ok {
			fmt.Fprintf(out, "peer %q: no valid samples\n", name)
			continue
		}
		fmt.Fprintf(out, "peer %q: offset=%v (from line %d, delay=%v) window=%d jitter=%v\n",
			name, best.Offset, best.Line, best.Delay, len(p.Samples()), p.Jitter())
	}
	return nil
}
