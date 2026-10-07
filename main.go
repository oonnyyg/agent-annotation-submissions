package main

import (
	"flag"
	"fmt"
	"math/big"
	"os"
)

func ratStr(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return r.FloatString(6)
}

func run(configPath, eventsPath string, out *os.File) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	arrivals, err := LoadArrivals(eventsPath, cfg)
	if err != nil {
		return err
	}
	res, err := Simulate(cfg, arrivals)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "=== timeline (link rate %d B/s) ===\n", cfg.LinkRateBytesPerSec)
	for _, s := range res.Sent {
		fmt.Fprintf(out, "SEND stream=%s packet=%s len=%d arrive=%s start=%s finish=%s wait=%s\n",
			s.Stream, s.Packet, s.Length, ratStr(s.Arrive), ratStr(s.Start), ratStr(s.Finish),
			ratStr(new(big.Rat).Sub(s.Start, s.Arrive)))
	}
	for _, d := range res.Dropped {
		fmt.Fprintf(out, "DROP stream=%s packet=%s len=%d time=%s reason=%s\n",
			d.Stream, d.Packet, d.Length, ratStr(d.Time), d.Reason)
	}

	fmt.Fprintln(out, "=== per-stream stats ===")
	var totAdmit, totSent, totDrop int64
	for _, id := range res.Order {
		st := res.Stats[id]
		avg := "-"
		if st.Sent > 0 {
			avg = ratStr(new(big.Rat).Quo(st.TotalWait, big.NewRat(int64(st.Sent), 1)))
		}
		fmt.Fprintf(out, "stream=%s admitted=%d(%dB) sent=%d(%dB) dropped=%d(%dB) avgWait=%ss maxWait=%ss\n",
			id, st.Admitted, st.AdmittedByte, st.Sent, st.SentBytes,
			st.Dropped, st.DroppedBytes, avg, ratStr(st.MaxWait))
		totAdmit += st.AdmittedByte
		totSent += st.SentBytes
		totDrop += st.DroppedBytes
	}
	fmt.Fprintln(out, "=== reconciliation ===")
	fmt.Fprintf(out, "admitted=%dB sent=%dB dropped=%dB arrived=%dB balanced=%v finished=%ss\n",
		totAdmit, totSent, totDrop, totAdmit+totDrop, totAdmit == totSent, ratStr(res.Finished))
	return nil
}

func main() {
	config := flag.String("config", "config.json", "JSON stream/link configuration")
	events := flag.String("events", "arrivals.csv", "CSV arrival events (time,stream,packet,length)")
	flag.Parse()
	if err := run(*config, *events, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
