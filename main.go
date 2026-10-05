// Command call-auction-matcher is a local trading drill tool. It reads
// JSONL requests from stdin and writes JSONL responses to stdout. It is
// a simulation only: it never connects to any exchange or fund account.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"example.com/call-auction-matcher/auction"
)

type request struct {
	Type  string `json:"type"`            // order | cancel | preview | execute
	ID    string `json:"id,omitempty"`    // order id (order/cancel)
	Side  string `json:"side,omitempty"`  // buy | sell (order)
	Price int64  `json:"price,omitempty"` // positive integer cents (order)
	Qty   int64  `json:"qty,omitempty"`   // positive integer (order)
	Ref   int64  `json:"reference"`       // positive reference price (preview/execute)
	Batch string `json:"batch,omitempty"` // unique batch id (execute)
}

type response struct {
	Type    string          `json:"type"` // ack | auction | error
	Seq     int64           `json:"seq,omitempty"`
	ID      string          `json:"id,omitempty"`
	Message string          `json:"message,omitempty"`
	Result  *auction.Result `json:"result,omitempty"`
}

func main() {
	book := auction.NewBook()
	exec := auction.NewExecutor(book)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	emit := func(r response) {
		b, _ := json.Marshal(r)
		fmt.Fprintln(out, string(b))
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(raw, &req); err != nil {
			emit(response{Type: "error", Message: fmt.Sprintf("line %d: invalid JSON: %v", line, err)})
			continue
		}
		emit(handle(book, exec, req))
	}
	if err := sc.Err(); err != nil {
		emit(response{Type: "error", Message: err.Error()})
	}
}

func handle(book *auction.Book, exec *auction.Executor, req request) response {
	switch req.Type {
	case "order":
		o, err := book.AddOrder(req.ID, auction.Side(req.Side), req.Price, req.Qty)
		if err != nil {
			return response{Type: "error", ID: req.ID, Message: err.Error()}
		}
		return response{Type: "ack", ID: o.ID, Seq: o.Seq}
	case "cancel":
		if err := book.CancelOrder(req.ID); err != nil {
			return response{Type: "error", ID: req.ID, Message: err.Error()}
		}
		return response{Type: "ack", ID: req.ID}
	case "preview":
		r, err := book.Preview(req.Ref)
		if err != nil {
			return response{Type: "error", Message: err.Error()}
		}
		return response{Type: "auction", Result: r}
	case "execute":
		r, err := exec.Execute(req.Batch, req.Ref)
		if err != nil {
			return response{Type: "error", Message: err.Error()}
		}
		return response{Type: "auction", Result: r}
	default:
		return response{Type: "error", Message: fmt.Sprintf("unknown request type %q", req.Type)}
	}
}
