// Package match implements a three-way match engine for procurement:
// purchase orders, goods receipts, returns and supplier invoices.
package match

import (
	"errors"
	"fmt"
	"sort"
)

// OrderLine is one line of a purchase order.
type OrderLine struct {
	SKU       string `json:"sku"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"` // integer cents, tax excluded
}

// Order is a purchase order identified by ID.
type Order struct {
	ID       string
	Supplier string
	Lines    map[int]*OrderLine
}

// Receipt is a goods receipt batch against an order line.
type Receipt struct {
	ID        string
	OrderID   string
	Line      int
	Qty       int64
	Date      string // business date, YYYY-MM-DD
	Returned  int64
	Allocated int64 // quantity frozen by non-voided invoices
}

// Available returns the quantity that can still be returned or invoiced.
func (r *Receipt) Available() int64 { return r.Qty - r.Returned - r.Allocated }

// InvoiceLine is one line of a supplier invoice.
type InvoiceLine struct {
	OrderID   string `json:"order_id"`
	Line      int    `json:"line"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}

// Allocation records how many units of a receipt batch an invoice froze.
type Allocation struct {
	ReceiptID string `json:"receipt_id"`
	OrderID   string `json:"order_id"`
	Line      int    `json:"line"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}

// Invoice is a matched supplier invoice. Allocations are frozen at match
// time and never recomputed, even if earlier receipts are recorded later.
type Invoice struct {
	ID       string
	Supplier string
	Lines    []InvoiceLine
	Allocs   []Allocation
	Amount   int64 // total payable in cents
	Voided   bool
}

// Engine holds all state. Use NewEngine.
type Engine struct {
	Orders   map[string]*Order
	Receipts map[string]*Receipt
	Invoices map[string]*Invoice
}

func NewEngine() *Engine {
	return &Engine{
		Orders:   map[string]*Order{},
		Receipts: map[string]*Receipt{},
		Invoices: map[string]*Invoice{},
	}
}

// CreateOrder registers a purchase order.
func (e *Engine) CreateOrder(id, supplier string, lines map[int]OrderLine) error {
	if id == "" || supplier == "" {
		return errors.New("order id and supplier are required")
	}
	if _, ok := e.Orders[id]; ok {
		return fmt.Errorf("order %q already exists", id)
	}
	if len(lines) == 0 {
		return errors.New("order must have at least one line")
	}
	o := &Order{ID: id, Supplier: supplier, Lines: map[int]*OrderLine{}}
	for n, l := range lines {
		if l.SKU == "" || l.Qty <= 0 || l.UnitPrice < 0 {
			return fmt.Errorf("order line %d: sku, positive qty and non-negative price required", n)
		}
		cp := l
		o.Lines[n] = &cp
	}
	e.Orders[id] = o
	return nil
}

// Receive records a goods receipt batch. Cumulative net received quantity
// per order line must not exceed the ordered quantity.
func (e *Engine) Receive(id, orderID string, line int, qty int64, date string) error {
	if id == "" || date == "" {
		return errors.New("receipt id and date are required")
	}
	if qty <= 0 {
		return errors.New("receipt qty must be a positive integer")
	}
	if _, ok := e.Receipts[id]; ok {
		return fmt.Errorf("receipt %q already exists", id)
	}
	o, ok := e.Orders[orderID]
	if !ok {
		return fmt.Errorf("order %q not found", orderID)
	}
	ol, ok := o.Lines[line]
	if !ok {
		return fmt.Errorf("order %q has no line %d", orderID, line)
	}
	var net int64
	for _, r := range e.Receipts {
		if r.OrderID == orderID && r.Line == line {
			net += r.Qty - r.Returned
		}
	}
	if net+qty > ol.Qty {
		return fmt.Errorf("receipt would exceed ordered qty: net %d + %d > %d", net, qty, ol.Qty)
	}
	e.Receipts[id] = &Receipt{ID: id, OrderID: orderID, Line: line, Qty: qty, Date: date}
	return nil
}

// Return sends goods back against an existing receipt batch. Only the
// quantity not yet frozen by invoices can be returned.
func (e *Engine) Return(id, receiptID string, qty int64) error {
	if id == "" {
		return errors.New("return id is required")
	}
	if qty <= 0 {
		return errors.New("return qty must be a positive integer")
	}
	r, ok := e.Receipts[receiptID]
	if !ok {
		return fmt.Errorf("receipt %q not found", receiptID)
	}
	if qty > r.Available() {
		return fmt.Errorf("cannot return %d: only %d not frozen by invoices", qty, r.Available())
	}
	r.Returned += qty
	return nil
}

// MatchInvoice validates and matches an invoice. Allocation is per order
// line across receipt batches ordered by (date, receipt id). The whole
// invoice is rejected if any line is invalid or lacks received quantity.
func (e *Engine) MatchInvoice(id, supplier string, lines []InvoiceLine) (*Invoice, error) {
	if id == "" || supplier == "" {
		return nil, errors.New("invoice id and supplier are required")
	}
	if _, ok := e.Invoices[id]; ok {
		return nil, fmt.Errorf("invoice %q already exists", id)
	}
	if len(lines) == 0 {
		return nil, errors.New("invoice must have at least one line")
	}
	inv := &Invoice{ID: id, Supplier: supplier, Lines: lines}
	for _, l := range lines {
		if l.Qty <= 0 {
			return nil, fmt.Errorf("invoice line %+v: qty must be positive", l)
		}
		o, ok := e.Orders[l.OrderID]
		if !ok {
			return nil, fmt.Errorf("order %q not found", l.OrderID)
		}
		if o.Supplier != supplier {
			return nil, fmt.Errorf("order %q belongs to supplier %q, not %q", l.OrderID, o.Supplier, supplier)
		}
		ol, ok := o.Lines[l.Line]
		if !ok {
			return nil, fmt.Errorf("order %q has no line %d", l.OrderID, l.Line)
		}
		if l.UnitPrice != ol.UnitPrice {
			return nil, fmt.Errorf("order %q line %d: price %d does not match order price %d", l.OrderID, l.Line, l.UnitPrice, ol.UnitPrice)
		}
		// Collect candidate batches, oldest first.
		var batches []*Receipt
		for _, r := range e.Receipts {
			if r.OrderID == l.OrderID && r.Line == l.Line && r.Available() > 0 {
				batches = append(batches, r)
			}
		}
		sort.Slice(batches, func(i, j int) bool {
			if batches[i].Date != batches[j].Date {
				return batches[i].Date < batches[j].Date
			}
			return batches[i].ID < batches[j].ID
		})
		remaining := l.Qty
		for _, b := range batches {
			if remaining == 0 {
				break
			}
			take := b.Available()
			if take > remaining {
				take = remaining
			}
			inv.Allocs = append(inv.Allocs, Allocation{
				ReceiptID: b.ID, OrderID: l.OrderID, Line: l.Line,
				Qty: take, UnitPrice: l.UnitPrice,
			})
			remaining -= take
		}
		if remaining > 0 {
			return nil, fmt.Errorf("order %q line %d: only %d received available, invoice needs %d", l.OrderID, l.Line, l.Qty-remaining, l.Qty)
		}
		inv.Amount += l.Qty * l.UnitPrice
	}
	// All lines valid: commit allocations.
	for _, a := range inv.Allocs {
		e.Receipts[a.ReceiptID].Allocated += a.Qty
	}
	e.Invoices[id] = inv
	return inv, nil
}

// VoidInvoice reverses a matched invoice, releasing its frozen allocations
// exactly once. The invoice record stays queryable as voided history.
func (e *Engine) VoidInvoice(invoiceID string) error {
	inv, ok := e.Invoices[invoiceID]
	if !ok {
		return fmt.Errorf("invoice %q not found", invoiceID)
	}
	if inv.Voided {
		return fmt.Errorf("invoice %q already voided", invoiceID)
	}
	for _, a := range inv.Allocs {
		e.Receipts[a.ReceiptID].Allocated -= a.Qty
	}
	inv.Voided = true
	return nil
}
