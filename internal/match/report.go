package match

import "fmt"

// LineBalance reports the three-way match position of one order line.
type LineBalance struct {
	OrderID            string `json:"order_id"`
	Line               int    `json:"line"`
	SKU                string `json:"sku"`
	Supplier           string `json:"supplier"`
	UnitPrice          int64  `json:"unit_price"`
	Ordered            int64  `json:"ordered"`
	ReceivedNet        int64  `json:"received_net"`        // received minus returned
	Unreceived         int64  `json:"unreceived"`          // ordered minus net received
	ReceivedUninvoiced int64  `json:"received_uninvoiced"` // net received not frozen by valid invoices
	InvoicedValid      int64  `json:"invoiced_valid"`      // frozen by non-voided invoices
	PayableCents       int64  `json:"payable_cents"`       // InvoicedValid * UnitPrice
}

// Balance computes the balance report for an order line.
// Conservation: Ordered == Unreceived + ReceivedUninvoiced + InvoicedValid.
func (e *Engine) Balance(orderID string, line int) (*LineBalance, error) {
	o, ok := e.Orders[orderID]
	if !ok {
		return nil, fmt.Errorf("order %q not found", orderID)
	}
	ol, ok := o.Lines[line]
	if !ok {
		return nil, fmt.Errorf("order %q has no line %d", orderID, line)
	}
	b := &LineBalance{
		OrderID: orderID, Line: line, SKU: ol.SKU, Supplier: o.Supplier,
		UnitPrice: ol.UnitPrice, Ordered: ol.Qty,
	}
	for _, r := range e.Receipts {
		if r.OrderID == orderID && r.Line == line {
			b.ReceivedNet += r.Qty - r.Returned
			b.InvoicedValid += r.Allocated
		}
	}
	b.ReceivedUninvoiced = b.ReceivedNet - b.InvoicedValid
	b.Unreceived = b.Ordered - b.ReceivedNet
	b.PayableCents = b.InvoicedValid * b.UnitPrice
	return b, nil
}

// InvoiceDetail is the query result for one invoice, valid or voided.
type InvoiceDetail struct {
	ID       string        `json:"id"`
	Supplier string        `json:"supplier"`
	Amount   int64         `json:"amount"`
	Voided   bool          `json:"voided"`
	Lines    []InvoiceLine `json:"lines"`
	Allocs   []Allocation  `json:"allocations"`
}

// InvoiceReport returns the frozen allocation detail of an invoice.
func (e *Engine) InvoiceReport(invoiceID string) (*InvoiceDetail, error) {
	inv, ok := e.Invoices[invoiceID]
	if !ok {
		return nil, fmt.Errorf("invoice %q not found", invoiceID)
	}
	return &InvoiceDetail{
		ID: inv.ID, Supplier: inv.Supplier, Amount: inv.Amount,
		Voided: inv.Voided, Lines: inv.Lines, Allocs: inv.Allocs,
	}, nil
}
