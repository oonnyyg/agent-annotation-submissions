package auction

import (
	"errors"
	"sort"
)

// match builds the trade list for the given auction price without
// mutating the book. Buys sort by price desc then seq asc; sells by
// price asc then seq asc. Orders pair up in that order, each fill at p,
// never exceeding either side's remaining quantity.
func (b *Book) match(p int64) []Trade {
	var buys, sells []*Order
	for _, o := range b.liveOrders() {
		if o.Side == Buy && o.Price >= p {
			buys = append(buys, o)
		}
		if o.Side == Sell && o.Price <= p {
			sells = append(sells, o)
		}
	}
	sort.Slice(buys, func(i, j int) bool {
		if buys[i].Price != buys[j].Price {
			return buys[i].Price > buys[j].Price
		}
		return buys[i].Seq < buys[j].Seq
	})
	sort.Slice(sells, func(i, j int) bool {
		if sells[i].Price != sells[j].Price {
			return sells[i].Price < sells[j].Price
		}
		return sells[i].Seq < sells[j].Seq
	})
	var trades []Trade
	i, j := 0, 0
	bi, si := int64(0), int64(0)
	for i < len(buys) && j < len(sells) {
		bq := buys[i].Remaining - bi
		sq := sells[j].Remaining - si
		q := min(bq, sq)
		trades = append(trades, Trade{BuyID: buys[i].ID, SellID: sells[j].ID, Price: p, Qty: q})
		bi += q
		si += q
		if bi == buys[i].Remaining {
			i++
			bi = 0
		}
		if si == sells[j].Remaining {
			j++
			si = 0
		}
	}
	return trades
}

// Preview returns the auction result for the current book snapshot
// without consuming any order quantity.
func (b *Book) Preview(ref int64) (*Result, error) {
	if ref <= 0 {
		return nil, errors.New("reference price must be a positive integer (cents)")
	}
	price, _, ok := b.SelectPrice(ref)
	r := &Result{Remaining: b.snapshot()}
	if !ok {
		r.Trades = []Trade{}
		return r, nil
	}
	r.Priced = true
	r.Price = price
	r.Trades = b.match(price)
	for _, t := range r.Trades {
		r.Volume += t.Qty
	}
	return r, nil
}

// snapshot lists live orders with their remaining quantities.
func (b *Book) snapshot() []Order {
	live := b.liveOrders()
	out := make([]Order, 0, len(live))
	for _, o := range live {
		out = append(out, *o)
	}
	return out
}
