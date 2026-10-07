package main

import (
	"fmt"
	"math/big"
)

// Ledger replays events for a single asset. Rejected events consume a
// sequence number but leave the state untouched.
type Ledger struct {
	lots    []*Lot
	sales   []Sale
	seenIDs map[string]bool
	lastTS  string
	seq     int // next event index
}

func NewLedger() *Ledger {
	return &Ledger{seenIDs: map[string]bool{}}
}

// Apply validates and applies one event. On any error the state is
// unchanged, but the sequence number still advances.
func (l *Ledger) Apply(ev Event) error {
	idx := l.seq
	l.seq++
	if ev.ID == "" {
		return fmt.Errorf("event %d: missing id", idx)
	}
	if l.seenIDs[ev.ID] {
		return fmt.Errorf("event %d: duplicate id %q", idx, ev.ID)
	}
	if ev.Timestamp == "" {
		return fmt.Errorf("event %d: missing ts", idx)
	}
	if l.lastTS != "" && ev.Timestamp < l.lastTS {
		return fmt.Errorf("event %d: ts %q goes backwards", idx, ev.Timestamp)
	}
	if ev.FeeCents < 0 || ev.AmountCents < 0 {
		return fmt.Errorf("event %d: negative amount or fee", idx)
	}
	var err error
	switch ev.Type {
	case "buy":
		err = l.applyBuy(idx, ev)
	case "sell":
		err = l.applySell(idx, ev)
	case "split":
		err = l.applySplit(idx, ev)
	default:
		err = fmt.Errorf("event %d: unknown type %q", idx, ev.Type)
	}
	if err != nil {
		return err
	}
	l.seenIDs[ev.ID] = true
	l.lastTS = ev.Timestamp
	return nil
}

func (l *Ledger) applyBuy(idx int, ev Event) error {
	qty, err := parseRat(ev.Qty, "qty")
	if err != nil {
		return fmt.Errorf("event %d: %w", idx, err)
	}
	if qty.Sign() <= 0 {
		return fmt.Errorf("event %d: buy qty must be positive", idx)
	}
	cost := new(big.Rat).Add(ratCents(ev.AmountCents), ratCents(ev.FeeCents))
	l.lots = append(l.lots, &Lot{
		ID:   ev.ID,
		Open: qty,
		Cost: cost,
		Seq:  idx,
	})
	return nil
}

func (l *Ledger) applySplit(idx int, ev Event) error {
	ratio, err := parseRat(ev.Ratio, "ratio")
	if err != nil {
		return fmt.Errorf("event %d: %w", idx, err)
	}
	if ratio.Sign() <= 0 {
		return fmt.Errorf("event %d: split ratio must be positive", idx)
	}
	for _, lot := range l.lots {
		if lot.Closed {
			continue
		}
		lot.Open = new(big.Rat).Mul(lot.Open, ratio)
	}
	return nil
}

// plannedDraw is a validated per-lot consumption plan.
type plannedDraw struct {
	lot *Lot
	qty *big.Rat
}

// applySell validates the whole sale first, then mutates lots.
func (l *Ledger) applySell(idx int, ev Event) error {
	qty, err := parseRat(ev.Qty, "qty")
	if err != nil {
		return fmt.Errorf("event %d: %w", idx, err)
	}
	if qty.Sign() <= 0 {
		return fmt.Errorf("event %d: sell qty must be positive", idx)
	}
	method := ev.Method
	if method == "" {
		method = "fifo"
	}
	var plan []plannedDraw
	switch method {
	case "fifo":
		plan, err = l.planFIFO(qty)
	case "explicit":
		plan, err = l.planExplicit(idx, ev, qty)
	default:
		err = fmt.Errorf("event %d: unknown method %q", idx, method)
	}
	if err != nil {
		return err
	}
	proceeds := new(big.Rat).Sub(ratCents(ev.AmountCents), ratCents(ev.FeeCents))
	if proceeds.Sign() < 0 {
		return fmt.Errorf("event %d: fee exceeds sale amount", idx)
	}
	sale := Sale{Seq: idx, EventID: ev.ID, Qty: new(big.Rat).Set(qty), Proceeds: proceeds}
	realized := new(big.Rat)
	for _, d := range plan {
		// Cost consumed is proportional to the fraction of the lot sold.
		costOut := new(big.Rat).Mul(d.lot.Cost, new(big.Rat).Quo(d.qty, d.lot.Open))
		// Proceeds are allocated pro-rata by quantity across this sale's lots.
		share := new(big.Rat).Mul(proceeds, new(big.Rat).Quo(d.qty, qty))
		gain := new(big.Rat).Sub(share, costOut)
		sale.Sources = append(sale.Sources, SaleSource{
			LotID:    d.lot.ID,
			Qty:      new(big.Rat).Set(d.qty),
			Cost:     costOut,
			Proceeds: share,
			Realized: gain,
		})
		realized.Add(realized, gain)
		d.lot.Open.Sub(d.lot.Open, d.qty)
		d.lot.Cost.Sub(d.lot.Cost, costOut)
		if d.lot.Open.Sign() == 0 {
			d.lot.Closed = true
		}
	}
	sale.Realized = realized
	l.sales = append(l.sales, sale)
	return nil
}

func (l *Ledger) planFIFO(qty *big.Rat) ([]plannedDraw, error) {
	remaining := new(big.Rat).Set(qty)
	var plan []plannedDraw
	for _, lot := range l.lots {
		if lot.Closed || remaining.Sign() == 0 {
			continue
		}
		take := lot.Open
		if take.Cmp(remaining) > 0 {
			take = remaining
		}
		plan = append(plan, plannedDraw{lot: lot, qty: new(big.Rat).Set(take)})
		remaining.Sub(remaining, take)
	}
	if remaining.Sign() != 0 {
		return nil, fmt.Errorf("insufficient holdings: short %s", ratString(remaining))
	}
	return plan, nil
}

func (l *Ledger) planExplicit(idx int, ev Event, qty *big.Rat) ([]plannedDraw, error) {
	if len(ev.Allocations) == 0 {
		return nil, fmt.Errorf("event %d: explicit sell requires allocations", idx)
	}
	total := new(big.Rat)
	seen := map[string]bool{}
	var plan []plannedDraw
	for _, a := range ev.Allocations {
		aq, err := parseRat(a.Qty, "allocation qty")
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", idx, err)
		}
		if aq.Sign() <= 0 {
			return nil, fmt.Errorf("event %d: allocation qty must be positive", idx)
		}
		if seen[a.LotID] {
			return nil, fmt.Errorf("event %d: duplicate allocation for lot %q", idx, a.LotID)
		}
		seen[a.LotID] = true
		lot := l.findLot(a.LotID)
		if lot == nil {
			return nil, fmt.Errorf("event %d: unknown lot %q", idx, a.LotID)
		}
		if lot.Closed || lot.Open.Cmp(aq) < 0 {
			return nil, fmt.Errorf("event %d: lot %q has only %s open, need %s",
				idx, a.LotID, ratString(lot.Open), ratString(aq))
		}
		plan = append(plan, plannedDraw{lot: lot, qty: aq})
		total.Add(total, aq)
	}
	if total.Cmp(qty) != 0 {
		return nil, fmt.Errorf("event %d: allocations sum %s != sell qty %s",
			idx, ratString(total), ratString(qty))
	}
	return plan, nil
}

func (l *Ledger) findLot(id string) *Lot {
	for _, lot := range l.lots {
		if lot.ID == id {
			return lot
		}
	}
	return nil
}
