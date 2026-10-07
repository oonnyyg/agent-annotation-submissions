package main

import (
	"fmt"
	"math/big"
)

// Event is one JSONL record. Qty and Ratio accept decimal or "a/b"
// fraction strings; AmountCents and FeeCents are integer cents.
type Event struct {
	ID          string       `json:"id"`
	Timestamp   string       `json:"ts"`
	Type        string       `json:"type"` // buy | sell | split
	Qty         string       `json:"qty,omitempty"`
	AmountCents int64        `json:"amount_cents,omitempty"`
	FeeCents    int64        `json:"fee_cents,omitempty"`
	Method      string       `json:"method,omitempty"` // fifo (default) | explicit
	Allocations []Allocation `json:"allocations,omitempty"`
	Ratio       string       `json:"ratio,omitempty"` // split only
}

// Allocation pins part of a sell to a specific lot.
type Allocation struct {
	LotID string `json:"lot"`
	Qty   string `json:"qty"`
}

// Lot is a purchase lot tracked with exact rationals.
type Lot struct {
	ID     string
	Open   *big.Rat // remaining quantity
	Cost   *big.Rat // remaining cost basis in cents, exact
	Seq    int      // event index that created the lot
	Closed bool
}

// SaleSource records how much quantity/cost one lot contributed to a sale.
type SaleSource struct {
	LotID    string
	Qty      *big.Rat
	Cost     *big.Rat
	Proceeds *big.Rat
	Realized *big.Rat
}

// Sale is a recorded sell with its per-lot sources.
type Sale struct {
	Seq      int
	EventID  string
	Qty      *big.Rat
	Proceeds *big.Rat // amount minus fee
	Realized *big.Rat
	Sources  []SaleSource
}

func ratCents(c int64) *big.Rat { return new(big.Rat).SetInt64(c) }

func ratString(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return r.RatString()
}

func parseRat(s, field string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid %s %q: want decimal or a/b fraction", field, s)
	}
	return r, nil
}
