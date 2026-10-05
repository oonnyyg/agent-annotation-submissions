// Package auction implements a call auction matcher for local trading
// drills only. It never connects to any exchange or fund account.
package auction

import (
	"errors"
	"fmt"
	"sort"
)

// Side is the direction of an order.
type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// Order is a resting limit order.
type Order struct {
	ID        string `json:"id"`
	Side      Side   `json:"side"`
	Price     int64  `json:"price"` // positive integer cents
	Qty       int64  `json:"qty"`   // original quantity
	Seq       int64  `json:"seq"`   // increasing acceptance sequence number
	Remaining int64  `json:"remaining"`
}

// Trade is a single matched fill at the auction price.
type Trade struct {
	BuyID  string `json:"buy_id"`
	SellID string `json:"sell_id"`
	Price  int64  `json:"price"`
	Qty    int64  `json:"qty"`
}

// Result is the outcome of one auction pricing/matching snapshot.
type Result struct {
	Priced    bool    `json:"priced"`
	Price     int64   `json:"price,omitempty"`
	Volume    int64   `json:"volume"`
	Trades    []Trade `json:"trades"`
	Remaining []Order `json:"remaining"`
}

// Book accumulates limit orders and runs call auctions over them.
type Book struct {
	orders  map[string]*Order // live orders
	usedIDs map[string]bool   // every id ever accepted (incl. cancelled)
	nextSeq int64
}

// NewBook creates an empty order book.
func NewBook() *Book {
	return &Book{orders: map[string]*Order{}, usedIDs: map[string]bool{}, nextSeq: 1}
}

var (
	ErrDuplicateID = errors.New("order id already used")
	ErrUnknownID   = errors.New("unknown order id")
)

// AddOrder validates and accepts a limit order, assigning it the next
// acceptance sequence number. Invalid requests leave the book unchanged.
func (b *Book) AddOrder(id string, side Side, price, qty int64) (*Order, error) {
	if id == "" {
		return nil, errors.New("order id must not be empty")
	}
	if b.usedIDs[id] {
		return nil, fmt.Errorf("%w: %s", ErrDuplicateID, id)
	}
	if side != Buy && side != Sell {
		return nil, fmt.Errorf("invalid side %q", side)
	}
	if price <= 0 {
		return nil, errors.New("price must be a positive integer (cents)")
	}
	if qty <= 0 {
		return nil, errors.New("quantity must be a positive integer")
	}
	o := &Order{ID: id, Side: side, Price: price, Qty: qty, Seq: b.nextSeq, Remaining: qty}
	b.nextSeq++
	b.orders[id] = o
	b.usedIDs[id] = true
	return o, nil
}

// CancelOrder removes a live order. The id stays used and cannot be reused.
func (b *Book) CancelOrder(id string) error {
	if _, ok := b.orders[id]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownID, id)
	}
	delete(b.orders, id)
	return nil
}

// liveOrders returns live orders with positive remaining quantity.
func (b *Book) liveOrders() []*Order {
	out := make([]*Order, 0, len(b.orders))
	for _, o := range b.orders {
		if o.Remaining > 0 {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// SelectPrice picks the auction price for the given positive reference
// price: maximize matchable volume, then minimize imbalance, then minimize
// distance to the reference, then prefer the lower price. It returns
// (price, volume, true) or (0, 0, false) when no price can match anything.
func (b *Book) SelectPrice(ref int64) (int64, int64, bool) {
	live := b.liveOrders()
	cand := map[int64]bool{}
	for _, o := range live {
		cand[o.Price] = true
	}
	bestPrice, bestVol := int64(0), int64(0)
	bestImb, bestDist := int64(0), int64(0)
	found := false
	for p := range cand {
		var buyVol, sellVol int64
		for _, o := range live {
			if o.Side == Buy && o.Price >= p {
				buyVol += o.Remaining
			}
			if o.Side == Sell && o.Price <= p {
				sellVol += o.Remaining
			}
		}
		vol := min(buyVol, sellVol)
		if vol == 0 {
			continue
		}
		imb := buyVol - sellVol
		if imb < 0 {
			imb = -imb
		}
		dist := p - ref
		if dist < 0 {
			dist = -dist
		}
		if !found || vol > bestVol ||
			(vol == bestVol && imb < bestImb) ||
			(vol == bestVol && imb == bestImb && dist < bestDist) ||
			(vol == bestVol && imb == bestImb && dist == bestDist && p < bestPrice) {
			found, bestPrice, bestVol, bestImb, bestDist = true, p, vol, imb, dist
		}
	}
	return bestPrice, bestVol, found
}
