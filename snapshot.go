package main

import "math/big"

// Snapshot is an immutable view of holdings and sales after event seq.
type Snapshot struct {
	AfterSeq int
	Lots     []Lot
	Sales    []SaleRecord
}

func cloneRat(r *big.Rat) *big.Rat { return new(big.Rat).Set(r) }

func cloneLot(l *Lot) Lot {
	return Lot{ID: l.ID, Seq: l.Seq, Qty: cloneRat(l.Qty), Cost: cloneRat(l.Cost)}
}

func cloneSale(s *SaleRecord) SaleRecord {
	out := SaleRecord{
		EventID:  s.EventID,
		Seq:      s.Seq,
		Qty:      cloneRat(s.Qty),
		NetCents: cloneRat(s.NetCents),
	}
	for _, src := range s.Sources {
		out.Sources = append(out.Sources, SaleSource{
			LotID:    src.LotID,
			Qty:      cloneRat(src.Qty),
			Cost:     cloneRat(src.Cost),
			Proceeds: cloneRat(src.Proceeds),
			Realized: cloneRat(src.Realized),
		})
	}
	return out
}

// SnapshotAt replays events[0..afterSeq] on a fresh ledger, so historical
// queries never mutate the live ledger. afterSeq is inclusive; use -1 for
// the empty initial state.
func SnapshotAt(events []Event, afterSeq int) (*Snapshot, []EventResult, error) {
	if afterSeq >= len(events) {
		return nil, nil, errSeqOutOfRange(afterSeq, len(events))
	}
	l := NewLedger()
	n := afterSeq + 1
	if n < 0 {
		n = 0
	}
	l.Replay(events[:n])
	snap := &Snapshot{AfterSeq: afterSeq}
	for _, lot := range l.lots {
		if lot.Qty.Sign() > 0 {
			snap.Lots = append(snap.Lots, cloneLot(lot))
		}
	}
	for _, s := range l.sales {
		snap.Sales = append(snap.Sales, cloneSale(s))
	}
	return snap, l.results, nil
}

type seqError struct {
	seq, total int
}

func (e seqError) Error() string {
	return "snapshot sequence out of range"
}

func errSeqOutOfRange(seq, total int) error { return seqError{seq: seq, total: total} }

// Results returns the per-event accept/reject outcomes.
func (l *Ledger) Results() []EventResult {
	out := make([]EventResult, len(l.results))
	copy(out, l.results)
	return out
}

// Snapshot returns a detached view of the ledger's current state.
func (l *Ledger) Snapshot() *Snapshot {
	snap := &Snapshot{AfterSeq: len(l.results) - 1}
	for _, lot := range l.lots {
		if lot.Qty.Sign() > 0 {
			snap.Lots = append(snap.Lots, cloneLot(lot))
		}
	}
	for _, s := range l.sales {
		snap.Sales = append(snap.Sales, cloneSale(s))
	}
	return snap
}

// Totals reconciles aggregate quantity and cost across open lots.
func (s *Snapshot) Totals() (qty, cost *big.Rat) {
	qty = new(big.Rat)
	cost = new(big.Rat)
	for _, lot := range s.Lots {
		qty.Add(qty, lot.Qty)
		cost.Add(cost, lot.Cost)
	}
	return qty, cost
}
