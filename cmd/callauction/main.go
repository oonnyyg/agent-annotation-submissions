// callauction 是本地集合竞价演练工具：从 stdin 逐行读取 JSONL 请求，
// 向 stdout 逐行输出 JSONL 响应。仅用于本题模拟，不连接交易所或资金账户。
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"example.com/call-auction-matcher/internal/auction"
)

type request struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Side  string `json:"side"`
	Price int64  `json:"price"`
	Qty   int64  `json:"qty"`
	Ref   int64  `json:"ref"`
	Batch string `json:"batch"`
}

func emit(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
}

func emitError(req request, msg string) {
	emit(map[string]any{"type": "error", "request_type": req.Type, "id": req.ID, "batch": req.Batch, "error": msg})
}

func main() {
	engine := auction.NewEngine(auction.NewBook())
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			emit(map[string]any{"type": "error", "error": "invalid JSON: " + err.Error()})
			continue
		}
		if err := handle(engine, req); err != nil {
			emitError(req, err.Error())
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}
}

func handle(engine *auction.Engine, req request) error {
	switch req.Type {
	case "place":
		order, err := engine.Book().Place(req.ID, auction.Side(req.Side), req.Price, req.Qty)
		if err != nil {
			return err
		}
		emit(map[string]any{"type": "placed", "id": order.ID, "side": string(order.Side), "price": order.Price, "qty": order.Qty, "seq": order.Seq})
	case "cancel":
		if err := engine.Book().Cancel(req.ID); err != nil {
			return err
		}
		emit(map[string]any{"type": "canceled", "id": req.ID})
	case "preview":
		result, err := engine.Preview(req.Ref)
		if err != nil {
			return err
		}
		emitResult("preview", req.Batch, result)
	case "execute":
		result, err := engine.Execute(req.Batch, req.Ref)
		if err != nil {
			if errors.Is(err, auction.ErrBatchRejected) {
				return err
			}
			return err
		}
		emitResult("executed", req.Batch, result)
	default:
		return fmt.Errorf("%w: unknown type %q", auction.ErrInvalidRequest, req.Type)
	}
	return nil
}

func emitResult(kind, batch string, result *auction.Result) {
	emit(map[string]any{
		"type":       kind,
		"batch":      batch,
		"matched":    result.Matched,
		"price":      result.Price,
		"total_qty":  result.TotalQty,
		"fills":      result.Fills,
		"remaining":  result.Remaining,
		"candidates": result.Candidates,
	})
}
