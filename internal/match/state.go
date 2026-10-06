// Package match implements a three-way match engine for procurement:
// purchase orders, goods receipts/returns and supplier invoices.
// Quantities are positive integers; money is integer cents (tax/FX excluded).
package match

import (
	"errors"
	"fmt"
	"sort"
)

// Line is a purchase order line: supplier, item, ordered qty and unit price.
type Line struct {
	OrderID   string `json:"order_id"`
	LineID    string `json:"line_id"`
	Supplier  string `json:"supplier"`
	Item      string `json:"item"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}

// Receipt is a goods receipt batch against an order line.
type Receipt struct {
	ID       string `json:"id"`
	OrderID  string `json:"order_id"`
	LineID   string `json:"line_id"`
	Date     string `json:"date"`
	Qty      int64  `json:"qty"`
	Returned int64  `json:"returned"`
	Invoiced int64  `json:"invoiced"`
}

// Available returns qty not returned and not yet consumed by invoices.
func (r *Receipt) Available() int64 { return r.Qty - r.Returned - r.Invoiced }

// InvoiceLine requests invoicing of qty at unitPrice on one order line.
type InvoiceLine struct {
	OrderID   string `json:"order_id"`
	LineID    string `json:"line_id"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}

// Allocation freezes how many units of a receipt batch an invoice consumed.
type Allocation struct {
	OrderID   string `json:"order_id"`
	LineID    string `json:"line_id"`
	ReceiptID string `json:"receipt_id"`
	Qty       int64  `json:"qty"`
}

// Invoice is a posted (or voided) supplier invoice with frozen allocations.
type Invoice struct {
	ID       string        `json:"id"`
	Supplier string        `json:"supplier"`
	Lines    []InvoiceLine `json:"lines"`
	Allocs   []Allocation  `json:"allocs"`
	Amount   int64         `json:"amount"`
	Voided   bool          `json:"voided"`
}

// State holds all domain data.
type State struct {
	Lines    map[string]*Line    // key: orderID/lineID
	Receipts map[string]*Receipt // key: receipt ID
	Invoices map[string]*Invoice // key: invoice ID
}

func NewState() *State {
	return &State{
		Lines:    map[string]*Line{},
		Receipts: map[string]*Receipt{},
		Invoices: map[string]*Invoice{},
	}
}

func lineKey(orderID, lineID string) string { return orderID + "/" + lineID }

var (
	ErrExists       = errors.New("id already exists")
	ErrNotFound     = errors.New("not found")
	ErrInvalidQty   = errors.New("quantity must be a positive integer")
	ErrOverReceive  = errors.New("cumulative net receipt exceeds ordered qty")
	ErrOverReturn   = errors.New("return exceeds uninvoiced received qty")
	ErrInsufficient = errors.New("insufficient uninvoiced received qty")
	ErrSupplier     = errors.New("invoice lines must share one supplier")
	ErrPrice        = errors.New("unit price differs from order line")
)

// CreateOrder registers an order line.
func (s *State) CreateOrder(l Line) error {
	if l.OrderID == "" || l.LineID == "" || l.Supplier == "" || l.Item == "" {
		return fmt.Errorf("order line requires order_id, line_id, supplier, item: %w", ErrNotFound)
	}
	if l.Qty <= 0 || l.UnitPrice < 0 {
		return ErrInvalidQty
	}
	k := lineKey(l.OrderID, l.LineID)
	if _, ok := s.Lines[k]; ok {
		return fmt.Errorf("order line %s: %w", k, ErrExists)
	}
	cp := l
	s.Lines[k] = &cp
	return nil
}

// Receive posts a goods receipt against an order line.
func (s *State) Receive(r Receipt) error {
	if r.ID == "" {
		return fmt.Errorf("receipt requires id: %w", ErrNotFound)
	}
	if r.Qty <= 0 {
		return ErrInvalidQty
	}
	l, ok := s.Lines[lineKey(r.OrderID, r.LineID)]
	if !ok {
		return fmt.Errorf("order line %s/%s: %w", r.OrderID, r.LineID, ErrNotFound)
	}
	if _, ok := s.Receipts[r.ID]; ok {
		return fmt.Errorf("receipt %s: %w", r.ID, ErrExists)
	}
	if s.netReceived(r.OrderID, r.LineID)+r.Qty > l.Qty {
		return fmt.Errorf("receipt %s: %w", r.ID, ErrOverReceive)
	}
	cp := r
	s.Receipts[r.ID] = &cp
	return nil
}

// Return posts a return against an original receipt batch. Only quantity not
// yet consumed by invoices may be returned.
func (s *State) Return(id, receiptID string, qty int64) error {
	if id == "" {
		return fmt.Errorf("return requires id: %w", ErrNotFound)
	}
	if qty <= 0 {
		return ErrInvalidQty
	}
	r, ok := s.Receipts[receiptID]
	if !ok {
		return fmt.Errorf("receipt %s: %w", receiptID, ErrNotFound)
	}
	if qty > r.Available() {
		return fmt.Errorf("return %s on receipt %s: %w", id, receiptID, ErrOverReturn)
	}
	r.Returned += qty
	return nil
}

func (s *State) netReceived(orderID, lineID string) int64 {
	var n int64
	for _, r := range s.Receipts {
		if r.OrderID == orderID && r.LineID == lineID {
			n += r.Qty - r.Returned
		}
	}
	return n
}

// PostInvoice validates and posts an invoice, allocating receipt batches in
// (date, receipt id) order per line. The whole invoice is rejected if any
// line is invalid or lacks sufficient uninvoiced received quantity.
func (s *State) PostInvoice(id string, lines []InvoiceLine) (*Invoice, error) {
	if id == "" || len(lines) == 0 {
		return nil, fmt.Errorf("invoice requires id and lines: %w", ErrNotFound)
	}
	if _, ok := s.Invoices[id]; ok {
		return nil, fmt.Errorf("invoice %s: %w", id, ErrExists)
	}
	supplier := ""
	var allocs []Allocation
	var amount int64
	for _, il := range lines {
		if il.Qty <= 0 {
			return nil, ErrInvalidQty
		}
		l, ok := s.Lines[lineKey(il.OrderID, il.LineID)]
		if !ok {
			return nil, fmt.Errorf("order line %s/%s: %w", il.OrderID, il.LineID, ErrNotFound)
		}
		if supplier == "" {
			supplier = l.Supplier
		} else if l.Supplier != supplier {
			return nil, fmt.Errorf("invoice %s: %w", id, ErrSupplier)
		}
		if il.UnitPrice != l.UnitPrice {
			return nil, fmt.Errorf("invoice %s line %s/%s: %w", id, il.OrderID, il.LineID, ErrPrice)
		}
		need := il.Qty
		for _, r := range s.sortedReceipts(il.OrderID, il.LineID) {
			if need == 0 {
				break
			}
			take := r.Available()
			if take > need {
				take = need
			}
			if take > 0 {
				allocs = append(allocs, Allocation{OrderID: il.OrderID, LineID: il.LineID, ReceiptID: r.ID, Qty: take})
				need -= take
			}
		}
		if need > 0 {
			return nil, fmt.Errorf("invoice %s line %s/%s short %d: %w", id, il.OrderID, il.LineID, need, ErrInsufficient)
		}
		amount += il.Qty * il.UnitPrice
	}
	inv := &Invoice{ID: id, Supplier: supplier, Lines: append([]InvoiceLine(nil), lines...), Allocs: allocs, Amount: amount}
	for _, a := range allocs {
		s.Receipts[a.ReceiptID].Invoiced += a.Qty
	}
	s.Invoices[id] = inv
	return inv, nil
}

// VoidInvoice releases the invoice's frozen allocations exactly once; the
// invoice record remains queryable.
func (s *State) VoidInvoice(id string) (*Invoice, error) {
	inv, ok := s.Invoices[id]
	if !ok {
		return nil, fmt.Errorf("invoice %s: %w", id, ErrNotFound)
	}
	if inv.Voided {
		return nil, fmt.Errorf("invoice %s already voided", id)
	}
	for _, a := range inv.Allocs {
		s.Receipts[a.ReceiptID].Invoiced -= a.Qty
	}
	inv.Voided = true
	return inv, nil
}

func (s *State) sortedReceipts(orderID, lineID string) []*Receipt {
	var rs []*Receipt
	for _, r := range s.Receipts {
		if r.OrderID == orderID && r.LineID == lineID {
			rs = append(rs, r)
		}
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].Date != rs[j].Date {
			return rs[i].Date < rs[j].Date
		}
		return rs[i].ID < rs[j].ID
	})
	return rs
}

// LineBalance reports the quantity buckets for an order line.
type LineBalance struct {
	OrderID    string `json:"order_id"`
	LineID     string `json:"line_id"`
	Ordered    int64  `json:"ordered"`
	Received   int64  `json:"received"`   // net received (receipts minus returns)
	Invoiced   int64  `json:"invoiced"`   // valid (non-voided) invoiced qty
	Uninvoiced int64  `json:"uninvoiced"` // received but not validly invoiced
	Unreceived int64  `json:"unreceived"` // ordered but not net received
	Payable    int64  `json:"payable"`    // cents: valid invoiced qty * unit price
}

// Balance computes the balance report for one order line.
func (s *State) Balance(orderID, lineID string) (LineBalance, error) {
	l, ok := s.Lines[lineKey(orderID, lineID)]
	if !ok {
		return LineBalance{}, fmt.Errorf("order line %s/%s: %w", orderID, lineID, ErrNotFound)
	}
	b := LineBalance{OrderID: orderID, LineID: lineID, Ordered: l.Qty}
	for _, r := range s.Receipts {
		if r.OrderID == orderID && r.LineID == lineID {
			b.Received += r.Qty - r.Returned
		}
	}
	for _, inv := range s.Invoices {
		if inv.Voided {
			continue
		}
		for _, il := range inv.Lines {
			if il.OrderID == orderID && il.LineID == lineID {
				b.Invoiced += il.Qty
			}
		}
	}
	b.Uninvoiced = b.Received - b.Invoiced
	b.Unreceived = b.Ordered - b.Received
	b.Payable = b.Invoiced * l.UnitPrice
	return b, nil
}
