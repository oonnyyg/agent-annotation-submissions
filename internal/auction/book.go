package auction

import (
	"errors"
	"sort"
)

// Side 表示订单方向。
type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// ErrInvalidRequest 表示请求非法；调用方保证非法请求不改变订单簿。
var ErrInvalidRequest = errors.New("invalid request")

// Order 是一笔限价订单。价格单位为分，数量为正整数。
type Order struct {
	ID       string `json:"id"`
	Side     Side   `json:"side"`
	Price    int64  `json:"price"`
	Qty      int64  `json:"qty"`
	Seq      int64  `json:"seq"`
	Filled   int64  `json:"filled"`
	Canceled bool   `json:"canceled"`
}

// Remaining 返回未成交余量。
func (o *Order) Remaining() int64 {
	return o.Qty - o.Filled
}

// Active 表示订单当前是否有效（未撤单且仍有余量）。
func (o *Order) Active() bool {
	return !o.Canceled && o.Filled < o.Qty
}

// Book 是本地演练用订单簿，不连接任何交易所或资金账户。
type Book struct {
	orders  map[string]*Order
	usedIDs map[string]bool
	nextSeq int64
}

// NewBook 创建空订单簿。
func NewBook() *Book {
	return &Book{
		orders:  make(map[string]*Order),
		usedIDs: make(map[string]bool),
	}
}

// Place 校验并受理一笔订单，成功后分配递增受理序号。
func (b *Book) Place(id string, side Side, price, qty int64) (*Order, error) {
	if id == "" {
		return nil, errors.Join(ErrInvalidRequest, errors.New("empty order id"))
	}
	if side != Buy && side != Sell {
		return nil, errors.Join(ErrInvalidRequest, errors.New("side must be buy or sell"))
	}
	if price <= 0 {
		return nil, errors.Join(ErrInvalidRequest, errors.New("price must be a positive integer"))
	}
	if qty <= 0 {
		return nil, errors.Join(ErrInvalidRequest, errors.New("qty must be a positive integer"))
	}
	if b.usedIDs[id] {
		return nil, errors.Join(ErrInvalidRequest, errors.New("order id already used and cannot be reused"))
	}
	b.nextSeq++
	order := &Order{
		ID:    id,
		Side:  side,
		Price: price,
		Qty:   qty,
		Seq:   b.nextSeq,
	}
	b.orders[id] = order
	b.usedIDs[id] = true
	return order, nil
}

// Cancel 撤销一笔仍有余量的有效订单；撤单后标识不可复用。
func (b *Book) Cancel(id string) error {
	order, ok := b.orders[id]
	if !ok {
		return errors.Join(ErrInvalidRequest, errors.New("unknown order id"))
	}
	if order.Canceled {
		return errors.Join(ErrInvalidRequest, errors.New("order already canceled"))
	}
	if order.Remaining() <= 0 {
		return errors.Join(ErrInvalidRequest, errors.New("order already fully filled"))
	}
	order.Canceled = true
	return nil
}

// ActiveOrders 返回当前全部有效订单，按受理序号升序。
func (b *Book) ActiveOrders() []*Order {
	var out []*Order
	for _, o := range b.orders {
		if o.Active() {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// allOrders 返回订单簿中全部订单（含已撤单、已全部成交），按受理序号升序。
func (b *Book) allOrders() []*Order {
	out := make([]*Order, 0, len(b.orders))
	for _, o := range b.orders {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}
