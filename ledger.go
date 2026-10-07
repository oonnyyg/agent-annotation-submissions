package main

import (
	"fmt"
	"math/big"
	"time"
)

// Lot is one open lot. Qty and Cost are exact rationals (shares / cents).
type Lot struct {
	ID   string   // originating buy event id
	Seq  int      // buy event index, defines FIFO order
	Qty  *big.Rat // remaining quantity
	Cost *big.Rat // remaining cost basis in cents
}

// SaleSource records how much of one lot funded a sale and its result.
type SaleSource struct {
	LotID    string
	Qty      *big.Rat // quantity taken from the lot
	Cost     *big.Rat // cost consumed (cents)
	Proceeds *big.Rat // share of net proceeds (cents)
	Realized *big.Rat // Proceeds - Cost (cents)
}

// SaleRecord is the outcome of an accepted sell event.
type SaleRecord struct {
	EventID  string
	Seq      int
	Qty      *big.Rat
	NetCents *big.Rat // total_cents - fee_cents
	Sources  []SaleSource
}

// EventResult describes what happened at one sequence number.
type EventResult struct {
	Seq      int
	ID       string
	Accepted bool
	Reason   string // rejection reason when !Accepted
}

// Ledger replays events for a single asset.
type Ledger struct {
	lots     []*Lot
	sales    []*SaleRecord
	results  []EventResult
	seenIDs  map[string]bool
	lastTime time.Time
	hasTime  bool
}

func NewLedger() *Ledger {
	return &Ledger{seenIDs: map[string]bool{}}
}

func ratFromCents(c int64) *big.Rat { return new(big.Rat).SetInt64(c) }

// Replay applies events in order. Every event occupies its sequence number;
// rejected events leave state untouched.
func (l *Ledger) Replay(events []Event) {
	for i := range events {
		l.apply(i, &events[i])
	}
}

func (l *Ledger) reject(seq int, id, reason string) {
	l.results = append(l.results, EventResult{Seq: seq, ID: id, Reason: reason})
}

func (l *Ledger) apply(seq int, ev *Event) {
	if err := l.checkHeader(ev); err != nil {
		l.reject(seq, ev.ID, err.Error())
		return
	}
	var err error
	switch ev.Type {
	case EventBuy:
		err = l.applyBuy(seq, ev)
	case EventSell:
		err = l.applySell(seq, ev)
	case EventSplit:
		err = l.applySplit(ev)
	default:
		err = fmt.Errorf("unknown event type %q", ev.Type)
	}
	if err != nil {
		l.reject(seq, ev.ID, err.Error())
		return
	}
	l.results = append(l.results, EventResult{Seq: seq, ID: ev.ID, Accepted: true})
}

// checkHeader validates id uniqueness and non-decreasing timestamps.
func (l *Ledger) checkHeader(ev *Event) error {
	if ev.ID == "" {
		return fmt.Errorf("missing id")
	}
	if l.seenIDs[ev.ID] {
		return fmt.Errorf("duplicate id %q", ev.ID)
	}
	ts, err := time.Parse(time.RFC3339, ev.Time)
	if err != nil {
		return fmt.Errorf("bad time %q: %v", ev.Time, err)
	}
	if l.hasTime && ts.Before(l.lastTime) {
		return fmt.Errorf("time goes backwards at %q", ev.ID)
	}
	l.seenIDs[ev.ID] = true
	l.lastTime = ts
	l.hasTime = true
	return nil
}

func requireCents(name string, p *int64) (int64, error) {
	if p == nil {
		return 0, fmt.Errorf("missing %s", name)
	}
	if *p < 0 {
		return 0, fmt.Errorf("%s must be non-negative", name)
	}
	return *p, nil
}

func (l *Ledger) applyBuy(seq int, ev *Event) error {
	qty, err := parseDecimalOrFraction(ev.Quantity)
	if err != nil || qty.Sign() <= 0 {
		return fmt.Errorf("buy quantity must be a positive decimal/fraction")
	}
	total, err := requireCents("total_cents", ev.TotalCents)
	if err != nil {
		return err
	}
	fee, err := requireCents("fee_cents", ev.FeeCents)
	if err != nil {
		return err
	}
	cost := ratFromCents(total)
	cost.Add(cost, ratFromCents(fee))
	l.lots = append(l.lots, &Lot{ID: ev.ID, Seq: seq, Qty: qty, Cost: cost})
	return nil
}

// portion pairs a lot with the quantity to consume from it.
type portion struct {
	lot *Lot
	qty *big.Rat
}

// planSell validates the sell fully and returns the portions to consume.
// No state is mutated here, so a failed sell never partially deducts.
func (l *Ledger) planSell(ev *Event, sellQty *big.Rat) ([]portion, error) {
	method := ev.Method
	if method == "" {
		method = "fifo"
	}
	switch method {
	case "fifo":
		var plans []portion
		remaining := new(big.Rat).Set(sellQty)
		for _, lot := range l.lots {
			if remaining.Sign() == 0 {
				break
			}
			if lot.Qty.Sign() == 0 {
				continue
			}
			take := new(big.Rat).Set(lot.Qty)
			if take.Cmp(remaining) > 0 {
				take.Set(remaining)
			}
			plans = append(plans, portion{lot: lot, qty: new(big.Rat).Set(take)})
			remaining.Sub(remaining, take)
		}
		if remaining.Sign() != 0 {
			return nil, fmt.Errorf("insufficient holdings: short %s", remaining.RatString())
		}
		return plans, nil
	case "specific":
		if len(ev.Allocations) == 0 {
			return nil, fmt.Errorf("specific sell requires allocations")
		}
		byID := map[string]*Lot{}
		for _, lot := range l.lots {
			byID[lot.ID] = lot
		}
		taken := map[string]*big.Rat{}
		var plans []portion
		total := new(big.Rat)
		for _, a := range ev.Allocations {
			lot, ok := byID[a.Lot]
			if !ok {
				return nil, fmt.Errorf("unknown lot %q", a.Lot)
			}
			q, err := parseDecimalOrFraction(a.Quantity)
			if err != nil || q.Sign() <= 0 {
				return nil, fmt.Errorf("allocation for lot %q must be positive", a.Lot)
			}
			sum := taken[a.Lot]
			if sum == nil {
				sum = new(big.Rat)
				taken[a.Lot] = sum
			}
			sum.Add(sum, q)
			if sum.Cmp(lot.Qty) > 0 {
				return nil, fmt.Errorf("lot %q has only %s remaining", a.Lot, lot.Qty.RatString())
			}
			total.Add(total, q)
			plans = append(plans, portion{lot: lot, qty: q})
		}
		if total.Cmp(sellQty) != 0 {
			return nil, fmt.Errorf("allocations sum %s != sell quantity %s", total.RatString(), sellQty.RatString())
		}
		return plans, nil
	default:
		return nil, fmt.Errorf("unknown sell method %q", ev.Method)
	}
}

func (l *Ledger) applySell(seq int, ev *Event) error {
	sellQty, err := parseDecimalOrFraction(ev.Quantity)
	if err != nil || sellQty.Sign() <= 0 {
		return fmt.Errorf("sell quantity must be a positive decimal/fraction")
	}
	total, err := requireCents("total_cents", ev.TotalCents)
	if err != nil {
		return err
	}
	fee, err := requireCents("fee_cents", ev.FeeCents)
	if err != nil {
		return err
	}
	plans, err := l.planSell(ev, sellQty)
	if err != nil {
		return err
	}
	net := ratFromCents(total - fee)
	rec := &SaleRecord{EventID: ev.ID, Seq: seq, Qty: new(big.Rat).Set(sellQty), NetCents: new(big.Rat).Set(net)}
	for _, p := range plans {
		// Cost consumed is proportional to the fraction of the lot sold.
		cost := new(big.Rat).Mul(p.lot.Cost, p.qty)
		cost.Quo(cost, p.lot.Qty)
		// Net proceeds are allocated pro-rata by quantity, exactly.
		proceeds := new(big.Rat).Mul(net, p.qty)
		proceeds.Quo(proceeds, sellQty)
		realized := new(big.Rat).Sub(proceeds, cost)
		p.lot.Cost.Sub(p.lot.Cost, cost)
		p.lot.Qty.Sub(p.lot.Qty, p.qty)
		rec.Sources = append(rec.Sources, SaleSource{
			LotID: p.lot.ID, Qty: p.qty, Cost: cost, Proceeds: proceeds, Realized: realized,
		})
	}
	l.sales = append(l.sales, rec)
	return nil
}

func (l *Ledger) applySplit(ev *Event) error {
	ratio, err := parseDecimalOrFraction(ev.Ratio)
	if err != nil || ratio.Sign() <= 0 {
		return fmt.Errorf("split ratio must be a positive decimal/fraction")
	}
	for _, lot := range l.lots {
		lot.Qty.Mul(lot.Qty, ratio)
	}
	return nil
}
