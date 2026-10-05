package auction

import (
	"fmt"
	"sort"
)

// Fill 是一笔逐对成交明细，买卖双方以同一价格成交 qty 数量。
type Fill struct {
	BuyOrderID  string `json:"buy_order_id"`
	SellOrderID string `json:"sell_order_id"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
	BuySeq      int64  `json:"buy_seq"`
	SellSeq     int64  `json:"sell_seq"`
}

// OrderRemaining 是执行后对外输出的订单余量快照。
type OrderRemaining struct {
	ID        string `json:"id"`
	Side      Side   `json:"side"`
	Price     int64  `json:"price"`
	Seq       int64  `json:"seq"`
	Qty       int64  `json:"qty"`
	Filled    int64  `json:"filled"`
	Remaining int64  `json:"remaining"`
	Canceled  bool   `json:"canceled"`
}

func sortBuyOrders(orders []*Order) {
	sort.SliceStable(orders, func(i, j int) bool {
		if orders[i].Price != orders[j].Price {
			return orders[i].Price > orders[j].Price
		}
		return orders[i].Seq < orders[j].Seq
	})
}

func sortSellOrders(orders []*Order) {
	sort.SliceStable(orders, func(i, j int) bool {
		if orders[i].Price != orders[j].Price {
			return orders[i].Price < orders[j].Price
		}
		return orders[i].Seq < orders[j].Seq
	})
}

// MatchAt 在 price 上对有效订单逐对撮合，返回成交明细。
// 买单按报价降序、卖单按报价升序，同价按受理序号；
// 全部以 price 成交，每次不超过任一订单剩余数量。
// 在订单副本上计算，不修改订单簿；应用撮合结果由 ApplyFills 完成。
func MatchAt(orders []*Order, price int64) []Fill {
	var buys, sells []*Order
	for _, o := range orders {
		cp := *o
		switch o.Side {
		case Buy:
			if cp.Price >= price && cp.Remaining() > 0 {
				buys = append(buys, &cp)
			}
		case Sell:
			if cp.Price <= price && cp.Remaining() > 0 {
				sells = append(sells, &cp)
			}
		}
	}
	sortBuyOrders(buys)
	sortSellOrders(sells)

	fills := []Fill{}
	i, j := 0, 0
	for i < len(buys) && j < len(sells) {
		buy, sell := buys[i], sells[j]
		qty := min(buy.Remaining(), sell.Remaining())
		if qty <= 0 {
			break
		}
		fills = append(fills, Fill{
			BuyOrderID:  buy.ID,
			SellOrderID: sell.ID,
			Price:       price,
			Qty:         qty,
			BuySeq:      buy.Seq,
			SellSeq:     sell.Seq,
		})
		buy.Filled += qty
		sell.Filled += qty
		if buy.Remaining() == 0 {
			i++
		}
		if sell.Remaining() == 0 {
			j++
		}
	}
	return fills
}

// ApplyFills 将撮合明细落到订单簿上的真实订单。
func ApplyFills(book *Book, fills []Fill) {
	for _, f := range fills {
		if o := book.orders[f.BuyOrderID]; o != nil {
			o.Filled += f.Qty
		}
		if o := book.orders[f.SellOrderID]; o != nil {
			o.Filled += f.Qty
		}
	}
}

// Snapshot 汇总订单簿中全部订单的余量，按受理序号升序。
func Snapshot(book *Book) []OrderRemaining {
	all := book.allOrders()
	out := make([]OrderRemaining, 0, len(all))
	for _, o := range all {
		out = append(out, OrderRemaining{
			ID:        o.ID,
			Side:      o.Side,
			Price:     o.Price,
			Seq:       o.Seq,
			Qty:       o.Qty,
			Filled:    o.Filled,
			Remaining: o.Remaining(),
			Canceled:  o.Canceled,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// VerifyConservation 核对成交明细：逐单数量不超余量、买卖双方总量相等。
func VerifyConservation(fills []Fill, before map[string]*Order) error {
	buyFilled := make(map[string]int64)
	sellFilled := make(map[string]int64)
	var totalBuy, totalSell int64
	for _, f := range fills {
		buyFilled[f.BuyOrderID] += f.Qty
		sellFilled[f.SellOrderID] += f.Qty
		totalBuy += f.Qty
		totalSell += f.Qty
		if f.Qty <= 0 {
			return fmt.Errorf("fill qty must be positive: %+v", f)
		}
	}
	if totalBuy != totalSell {
		return fmt.Errorf("total buy qty %d != total sell qty %d", totalBuy, totalSell)
	}
	for id, q := range buyFilled {
		o := before[id]
		if o == nil || o.Side != Buy || q > o.Remaining() {
			return fmt.Errorf("buy order %s filled %d exceeds remaining %v", id, q, o)
		}
	}
	for id, q := range sellFilled {
		o := before[id]
		if o == nil || o.Side != Sell || q > o.Remaining() {
			return fmt.Errorf("sell order %s filled %d exceeds remaining %v", id, q, o)
		}
	}
	return nil
}
