package match

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Request is one JSONL operation read from stdin.
type Request struct {
	Op       string          `json:"op"`
	ID       string          `json:"id"`
	Supplier string          `json:"supplier"`
	OrderID  string          `json:"order_id"`
	Line     int             `json:"line"`
	Qty      int64           `json:"qty"`
	Date     string          `json:"date"`
	Receipt  string          `json:"receipt_id"`
	Invoice  string          `json:"invoice_id"`
	Lines    json.RawMessage `json:"lines"`
}

// Response is one JSONL result written to stdout.
type Response struct {
	OK     bool        `json:"ok"`
	ID     string      `json:"id,omitempty"`
	Error  string      `json:"error,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

type opRecord struct {
	Hash string          `json:"hash"`
	Resp json.RawMessage `json:"resp"`
}

type logEntry struct {
	Req  json.RawMessage `json:"req"`
	Resp json.RawMessage `json:"resp"`
}

// Service wraps the engine with idempotency and durable persistence.
type Service struct {
	eng *Engine
	ops map[string]opRecord
	log *os.File
	enc *json.Encoder
}

const logName = "ops.jsonl"

// OpenService loads state by replaying the operation log in dir.
func OpenService(dir string) (*Service, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Service{eng: NewEngine(), ops: map[string]opRecord{}}
	path := filepath.Join(dir, logName)
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var e logEntry
			if err := json.Unmarshal(line, &e); err != nil {
				f.Close()
				return nil, fmt.Errorf("corrupt log: %w", err)
			}
			var req Request
			if err := json.Unmarshal(e.Req, &req); err != nil {
				f.Close()
				return nil, fmt.Errorf("corrupt log request: %w", err)
			}
			s.apply(&req) // deterministic replay rebuilds state
			s.ops[req.ID] = opRecord{Hash: hashReq(e.Req), Resp: e.Resp}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	lf, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	s.log = lf
	s.enc = json.NewEncoder(lf)
	return s, nil
}

// Close flushes and closes the operation log.
func (s *Service) Close() error { return s.log.Close() }

func hashReq(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Handle processes one raw JSONL request line and returns the response.
// Re-submitting the same id with identical content returns the original
// result; the same id with different content is rejected.
func (s *Service) Handle(raw json.RawMessage) Response {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return Response{OK: false, Error: "invalid JSON: " + err.Error()}
	}
	if req.ID == "" {
		return Response{OK: false, Error: "id is required"}
	}
	// Queries are read-only: never persisted, never idempotency-cached.
	if req.Op == "balance" || req.Op == "invoice_detail" {
		resp := s.apply(&req)
		resp.ID = req.ID
		return resp
	}
	h := hashReq(raw)
	if rec, ok := s.ops[req.ID]; ok {
		if rec.Hash != h {
			return Response{OK: false, ID: req.ID, Error: "id already used with different content"}
		}
		var resp Response
		if err := json.Unmarshal(rec.Resp, &resp); err != nil {
			return Response{OK: false, ID: req.ID, Error: "corrupt stored response"}
		}
		return resp
	}
	resp := s.apply(&req)
	resp.ID = req.ID
	rawResp, err := json.Marshal(resp)
	if err != nil {
		return Response{OK: false, ID: req.ID, Error: err.Error()}
	}
	entry, err := json.Marshal(logEntry{Req: raw, Resp: rawResp})
	if err != nil {
		return Response{OK: false, ID: req.ID, Error: err.Error()}
	}
	if err := s.enc.Encode(json.RawMessage(entry)); err != nil {
		return Response{OK: false, ID: req.ID, Error: "persist: " + err.Error()}
	}
	s.ops[req.ID] = opRecord{Hash: h, Resp: rawResp}
	return resp
}

// apply executes a request against the engine. Failures change no state.
func (s *Service) apply(req *Request) Response {
	fail := func(err error) Response { return Response{OK: false, Error: err.Error()} }
	switch req.Op {
	case "create_order":
		var lines map[int]OrderLine
		if err := json.Unmarshal(req.Lines, &lines); err != nil {
			return fail(fmt.Errorf("lines: %w", err))
		}
		if err := s.eng.CreateOrder(req.ID, req.Supplier, lines); err != nil {
			return fail(err)
		}
		return Response{OK: true, Result: map[string]string{"order": req.ID}}
	case "receive":
		if err := s.eng.Receive(req.ID, req.OrderID, req.Line, req.Qty, req.Date); err != nil {
			return fail(err)
		}
		return Response{OK: true, Result: map[string]string{"receipt": req.ID}}
	case "return":
		if err := s.eng.Return(req.ID, req.Receipt, req.Qty); err != nil {
			return fail(err)
		}
		return Response{OK: true, Result: map[string]string{"return": req.ID}}
	case "invoice":
		var lines []InvoiceLine
		if err := json.Unmarshal(req.Lines, &lines); err != nil {
			return fail(fmt.Errorf("lines: %w", err))
		}
		inv, err := s.eng.MatchInvoice(req.ID, req.Supplier, lines)
		if err != nil {
			return fail(err)
		}
		d, _ := s.eng.InvoiceReport(inv.ID)
		return Response{OK: true, Result: d}
	case "void_invoice":
		if err := s.eng.VoidInvoice(req.Invoice); err != nil {
			return fail(err)
		}
		d, _ := s.eng.InvoiceReport(req.Invoice)
		return Response{OK: true, Result: d}
	case "balance":
		b, err := s.eng.Balance(req.OrderID, req.Line)
		if err != nil {
			return fail(err)
		}
		return Response{OK: true, Result: b}
	case "invoice_detail":
		d, err := s.eng.InvoiceReport(req.Invoice)
		if err != nil {
			return fail(err)
		}
		return Response{OK: true, Result: d}
	default:
		return fail(fmt.Errorf("unknown op %q", req.Op))
	}
}
