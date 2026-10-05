package auction

import (
	"errors"
	"fmt"
)

// batch records an executed auction batch for idempotent retries.
type batch struct {
	ref    int64
	result *Result
}

// Executor runs auctions with idempotent batch semantics.
type Executor struct {
	book    *Book
	batches map[string]*batch
}

// NewExecutor wraps a book with batch-aware execution.
func NewExecutor(b *Book) *Executor {
	return &Executor{book: b, batches: map[string]*batch{}}
}

var ErrBatchParamMismatch = errors.New("batch id already used with different parameters")

// Execute runs the auction and deducts filled quantities. Retrying with
// the same batch id and the same reference price returns the original
// result without trading again; the same id with different parameters is
// rejected.
func (e *Executor) Execute(batchID string, ref int64) (*Result, error) {
	if batchID == "" {
		return nil, errors.New("batch id must not be empty")
	}
	if ref <= 0 {
		return nil, errors.New("reference price must be a positive integer (cents)")
	}
	if prev, ok := e.batches[batchID]; ok {
		if prev.ref != ref {
			return nil, fmt.Errorf("%w: %s", ErrBatchParamMismatch, batchID)
		}
		return prev.result, nil
	}
	r, err := e.book.Preview(ref)
	if err != nil {
		return nil, err
	}
	for _, t := range r.Trades {
		e.book.orders[t.BuyID].Remaining -= t.Qty
		e.book.orders[t.SellID].Remaining -= t.Qty
	}
	r.Remaining = e.book.snapshot()
	e.batches[batchID] = &batch{ref: ref, result: r}
	return r, nil
}
