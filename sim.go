package main

import (
	"fmt"
	"math/big"
	"sort"
)

// Simulate replays the arrivals over a deficit-round-robin scheduler.
func Simulate(cfg *Config, arrivals []Arrival) (*Result, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	n := len(cfg.Streams)
	order := make([]string, n)
	scfg := map[string]StreamConfig{}
	for i, s := range cfg.Streams {
		order[i] = s.ID
		scfg[s.ID] = s
	}
	sort.Strings(order)

	queues := map[string][]Packet{}
	waiting := map[string]int64{}
	deficit := map[string]int64{}
	stats := map[string]*StreamStats{}
	for _, id := range order {
		stats[id] = &StreamStats{ID: id, TotalWait: new(big.Rat), MaxWait: new(big.Rat)}
	}
	res := &Result{Stats: stats, Order: order, Finished: new(big.Rat)}

	sorted := append([]Arrival(nil), arrivals...)
	sortArrivals(sorted)
	next := 0

	admit := func(now *big.Rat) {
		for next < len(sorted) && sorted[next].Time.Cmp(now) <= 0 {
			a := sorted[next]
			next++
			sc := scfg[a.Stream]
			st := stats[a.Stream]
			if waiting[a.Stream]+a.Length > sc.MaxWaitingBytes {
				st.Dropped++
				st.DroppedBytes += a.Length
				res.Dropped = append(res.Dropped, DropRecord{
					Packet: a.Packet, Stream: a.Stream, Length: a.Length,
					Time: new(big.Rat).Set(a.Time),
					Reason: fmt.Sprintf("waiting room full: %d+%d exceeds cap %d",
						waiting[a.Stream], a.Length, sc.MaxWaitingBytes),
				})
				continue
			}
			queues[a.Stream] = append(queues[a.Stream], Packet{
				ID: a.Packet, Stream: a.Stream, Length: a.Length, Arrive: a.Time,
			})
			waiting[a.Stream] += a.Length
			st.Admitted++
			st.AdmittedByte += a.Length
		}
	}

	allIdle := func() bool {
		for _, id := range order {
			if len(queues[id]) > 0 {
				return false
			}
		}
		return true
	}

	now := new(big.Rat)
	rate := cfg.LinkRateBytesPerSec
	pos := 0 // index into order; round restarts at 0 after full idle

	for {
		if allIdle() {
			if next >= len(sorted) {
				break
			}
			if sorted[next].Time.Cmp(now) > 0 {
				now.Set(sorted[next].Time)
			}
			admit(now)
			pos = 0
			continue
		}
		id := order[pos]
		pos = (pos + 1) % n
		q := queues[id]
		if len(q) == 0 {
			continue // idle stream: no credit accrues
		}
		deficit[id] += scfg[id].Quantum
		for len(q) > 0 {
			head := q[0]
			if head.Length > deficit[id] {
				break // keep balance, move to next stream
			}
			start := new(big.Rat).Set(now)
			now.Add(now, big.NewRat(head.Length, rate))
			deficit[id] -= head.Length
			q = q[1:]
			queues[id] = q
			waiting[id] -= head.Length
			admit(now) // arrivals during transmission, admitted at finish time;
			// the in-flight packet no longer counts against waiting capacity
			st := stats[id]
			st.Sent++
			st.SentBytes += head.Length
			wait := new(big.Rat).Sub(start, head.Arrive)
			st.TotalWait.Add(st.TotalWait, wait)
			if wait.Cmp(st.MaxWait) > 0 {
				st.MaxWait.Set(wait)
			}
			res.Sent = append(res.Sent, SentRecord{
				Packet: head.ID, Stream: id, Length: head.Length,
				Arrive: head.Arrive, Start: start, Finish: new(big.Rat).Set(now),
			})
		}
		if len(q) == 0 {
			deficit[id] = 0 // queue drained: end visit, reset deficit
		}
	}
	res.Finished.Set(now)
	return res, nil
}
