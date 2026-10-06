package match

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Op is one business operation decoded from a JSONL input line.
type Op struct {
	Op        string        `json:"op"`
	OrderID   string        `json:"order_id,omitempty"`
	LineID    string        `json:"line_id,omitempty"`
	Supplier  string        `json:"supplier,omitempty"`
	Item      string        `json:"item,omitempty"`
	Qty       int64         `json:"qty,omitempty"`
	UnitPrice int64         `json:"unit_price,omitempty"`
	Date      string        `json:"date,omitempty"`
	ReceiptID string        `json:"receipt_id,omitempty"`
	ReturnID  string        `json:"return_id,omitempty"`
	InvoiceID string        `json:"invoice_id,omitempty"`
	Lines     []InvoiceLine `json:"lines,omitempty"`
}

// Result is one JSONL output line. Exactly one of the payload fields is set.
type Result struct {
	OK      bool          `json:"ok"`
	Op      string        `json:"op"`
	ID      string        `json:"id,omitempty"`
	Error   string        `json:"error,omitempty"`
	Balance *LineBalance  `json:"balance,omitempty"`
	Invoice *Invoice      `json:"invoice,omitempty"`
}

// idempotentOps mutate state and are deduplicated by (op, id).
var idempotentOps = map[string]bool{
	"create_order":  true,
	"receive":       true,
	"return":        true,
	"invoice":       true,
	"void_invoice":  true,
}

// Engine wraps State with idempotency and a JSONL op log for persistence.
type Engine struct {
	mu     sync.Mutex
	state  *State
	dir    string
	log    *os.File
	done   map[string]json.RawMessage // (op|id) -> canonical op content
	result map[string]Result          // (op|id) -> original result
}

// OpenEngine loads (or creates) the op log in dir and replays it.
func OpenEngine(dir string) (*Engine, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	e := &Engine{
		state:  NewState(),
		dir:    dir,
		done:   map[string]json.RawMessage{},
		result: map[string]Result{},
	}
	path := filepath.Join(dir, "ops.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	e.log = f
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		return nil, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var op Op
		if err := json.Unmarshal(line, &op); err != nil {
			return nil, fmt.Errorf("corrupt op log: %w", err)
		}
		res := e.apply(&op)
		if !res.OK {
			return nil, fmt.Errorf("replaying op %s/%s: %s", op.Op, opID(&op), res.Error)
		}
		e.record(&op, res)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, 2); err != nil {
		return nil, err
	}
	return e, nil
}

// Close flushes and closes the op log.
func (e *Engine) Close() error { return e.log.Close() }

func opID(op *Op) string {
	switch op.Op {
	case "create_order":
		return op.OrderID + "/" + op.LineID
	case "receive":
		return op.ReceiptID
	case "return":
		return op.ReturnID
	case "invoice", "void_invoice", "query_invoice":
		return op.InvoiceID
	}
	return ""
}

func (e *Engine) record(op *Op, res Result) {
	key := op.Op + "|" + opID(op)
	content, _ := json.Marshal(op)
	e.done[key] = content
	e.result[key] = res
}

// Execute runs one op and returns its result line.
func (e *Engine) Execute(op *Op) Result {
	e.mu.Lock()
	defer e.mu.Unlock()

	if idempotentOps[op.Op] {
		key := op.Op + "|" + opID(op)
		content, _ := json.Marshal(op)
		if prev, ok := e.done[key]; ok {
			if string(prev) == string(content) {
				return e.result[key] // duplicate: return original result
			}
			return Result{OK: false, Op: op.Op, ID: opID(op), Error: "conflicting resubmission for id " + opID(op)}
		}
	}
	res := e.apply(op)
	if !res.OK || !idempotentOps[op.Op] {
		return res // failures change nothing and are not recorded
	}
	line, _ := json.Marshal(op)
	if _, err := e.log.Write(append(line, '\n')); err != nil {
		return Result{OK: false, Op: op.Op, Error: "persist: " + err.Error()}
	}
	e.record(op, res)
	return res
}

func (e *Engine) apply(op *Op) Result {
	id := opID(op)
	switch op.Op {
	case "create_order":
		err := e.state.CreateOrder(Line{OrderID: op.OrderID, LineID: op.LineID, Supplier: op.Supplier, Item: op.Item, Qty: op.Qty, UnitPrice: op.UnitPrice})
		return Result{OK: err == nil, Op: op.Op, ID: id, Error: errStr(err)}
	case "receive":
		err := e.state.Receive(Receipt{ID: op.ReceiptID, OrderID: op.OrderID, LineID: op.LineID, Date: op.Date, Qty: op.Qty})
		return Result{OK: err == nil, Op: op.Op, ID: id, Error: errStr(err)}
	case "return":
		err := e.state.Return(op.ReturnID, op.ReceiptID, op.Qty)
		return Result{OK: err == nil, Op: op.Op, ID: id, Error: errStr(err)}
	case "invoice":
		inv, err := e.state.PostInvoice(op.InvoiceID, op.Lines)
		if err != nil {
			return Result{OK: false, Op: op.Op, ID: id, Error: err.Error()}
		}
		return Result{OK: true, Op: op.Op, ID: id, Invoice: inv}
	case "void_invoice":
		inv, err := e.state.VoidInvoice(op.InvoiceID)
		if err != nil {
			return Result{OK: false, Op: op.Op, ID: id, Error: err.Error()}
		}
		return Result{OK: true, Op: op.Op, ID: id, Invoice: inv}
	case "query_balance":
		b, err := e.state.Balance(op.OrderID, op.LineID)
		if err != nil {
			return Result{OK: false, Op: op.Op, Error: err.Error()}
		}
		return Result{OK: true, Op: op.Op, Balance: &b}
	case "query_invoice":
		inv, ok := e.state.Invoices[op.InvoiceID]
		if !ok {
			return Result{OK: false, Op: op.Op, ID: id, Error: ErrNotFound.Error()}
		}
		return Result{OK: true, Op: op.Op, ID: id, Invoice: inv}
	default:
		return Result{OK: false, Op: op.Op, Error: "unknown op " + op.Op}
	}
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
