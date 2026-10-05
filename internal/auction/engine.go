package auction

import (
	"errors"
	"fmt"
)

// Result 是一次预览或执行在相同快照下产生的竞价结果。
type Result struct {
	Matched    bool             `json:"matched"`
	Price      int64            `json:"price"`
	TotalQty   int64            `json:"total_qty"`
	Fills      []Fill           `json:"fills"`
	Remaining  []OrderRemaining `json:"remaining"`
	Candidates []PriceStats     `json:"candidates"`
}

// BatchRejected 表示相同批次号但参数与首次请求不同。
var ErrBatchRejected = errors.New("batch id reused with different parameters")

// Engine 在单个订单簿上提供预览与执行，并管理执行批次幂等。
type Engine struct {
	book    *Book
	batches map[string]storedBatch
}

type storedBatch struct {
	ref    int64
	result *Result
}

// NewEngine 创建绑定到 book 的竞价引擎。
func NewEngine(book *Book) *Engine {
	return &Engine{book: book, batches: make(map[string]storedBatch)}
}

// Book 返回引擎绑定的订单簿。
func (e *Engine) Book() *Book { return e.book }

func computeResult(book *Book, ref int64) *Result {
	active := book.ActiveOrders()
	stats := CandidateStats(active)
	res := &Result{Matched: false, Fills: []Fill{}, Remaining: Snapshot(book), Candidates: stats}
	price, ok := ChoosePrice(stats, ref)
	if !ok {
		return res
	}
	fills := MatchAt(active, price)
	if len(fills) == 0 {
		return res
	}
	var total int64
	for _, f := range fills {
		total += f.Qty
	}
	res.Matched = true
	res.Price = price
	res.TotalQty = total
	res.Fills = fills
	return res
}

// Preview 计算竞价结果但不扣减任何订单数量。
func (e *Engine) Preview(ref int64) (*Result, error) {
	if ref <= 0 {
		return nil, errors.Join(ErrInvalidRequest, errors.New("reference price must be a positive integer"))
	}
	return computeResult(e.book, ref), nil
}

// Execute 执行竞价并扣减成交数量；batchID 唯一，相同参数重试返回原结果，
// 同批次改参数拒绝。
func (e *Engine) Execute(batchID string, ref int64) (*Result, error) {
	if batchID == "" {
		return nil, errors.Join(ErrInvalidRequest, errors.New("empty batch id"))
	}
	if ref <= 0 {
		return nil, errors.Join(ErrInvalidRequest, errors.New("reference price must be a positive integer"))
	}
	if stored, ok := e.batches[batchID]; ok {
		if stored.ref != ref {
			return nil, fmt.Errorf("%w: batch %s", ErrBatchRejected, batchID)
		}
		return stored.result, nil
	}

	active := e.book.ActiveOrders()
	before := make(map[string]*Order, len(active))
	for _, o := range active {
		cp := *o
		before[o.ID] = &cp
	}
	result := computeResult(e.book, ref)
	if result.Matched {
		if err := VerifyConservation(result.Fills, before); err != nil {
			return nil, fmt.Errorf("conservation check failed: %w", err)
		}
		ApplyFills(e.book, result.Fills)
		result.Remaining = Snapshot(e.book)
	}
	e.batches[batchID] = storedBatch{ref: ref, result: result}
	return result, nil
}
