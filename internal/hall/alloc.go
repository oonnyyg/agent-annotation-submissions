package hall

import (
	"fmt"
	"sort"
)

// MaxBatch is the maximum number of orders allocated together.
const MaxBatch = 6

// OrderRequest describes one order to allocate in a batch.
type OrderRequest struct {
	ID   string
	Size int
	Pref Seat
}

// compareRuns lexicographically compares two seat sequences (row, then col).
func compareRuns(a, b []Seat) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i].Row != b[i].Row {
			return a[i].Row - b[i].Row
		}
		if a[i].Col != b[i].Col {
			return a[i].Col - b[i].Col
		}
	}
	return len(a) - len(b)
}

// compareChoice compares concatenated seat sequences of two assignments.
func compareChoice(a, b [][]Seat) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareRuns(a[i], b[i]); c != 0 {
			return c
		}
	}
	return len(a) - len(b)
}

// search finds the minimum-total-cost disjoint assignment for the batch.
// Ties break by lexicographic order of the concatenated seat sequence, with
// orders processed in the (already sorted) request order.
func search(cands [][][]Seat, costs [][]int) ([][]Seat, int, bool) {
	n := len(cands)
	used := map[string]bool{}
	best := -1
	var bestChoice [][]Seat
	cur := make([][]Seat, n)
	var dfs func(i, cost int)
	dfs = func(i, cost int) {
		if best >= 0 && cost > best {
			return
		}
		if i == n {
			if best < 0 || cost < best || compareChoice(cur, bestChoice) < 0 {
				best = cost
				bestChoice = make([][]Seat, n)
				for j, run := range cur {
					cp := make([]Seat, len(run))
					copy(cp, run)
					bestChoice[j] = cp
				}
			}
			return
		}
		for k, run := range cands[i] {
			ok := true
			for _, s := range run {
				if used[key(s)] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			for _, s := range run {
				used[key(s)] = true
			}
			cur[i] = run
			dfs(i+1, cost+costs[i][k])
			for _, s := range run {
				delete(used, key(s))
			}
		}
	}
	dfs(0, 0)
	return bestChoice, best, best >= 0
}

// Book allocates a batch of orders atomically. On any failure (duplicate IDs,
// invalid sizes, or no feasible joint assignment) nothing is changed.
func (h *Hall) Book(reqs []OrderRequest) ([]*Order, error) {
	if len(reqs) == 0 || len(reqs) > MaxBatch {
		return nil, fmt.Errorf("batch must contain 1..%d orders", MaxBatch)
	}
	seen := map[string]bool{}
	for _, r := range reqs {
		if r.ID == "" {
			return nil, fmt.Errorf("order id must not be empty")
		}
		if r.Size < 1 {
			return nil, fmt.Errorf("order %q: size must be positive", r.ID)
		}
		if seen[r.ID] || h.Orders[r.ID] != nil {
			return nil, fmt.Errorf("order %q: duplicate id", r.ID)
		}
		seen[r.ID] = true
	}
	// Tie-breaking requires orders in lexicographic ID order.
	sorted := make([]OrderRequest, len(reqs))
	copy(sorted, reqs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	reqs = sorted
	occ := h.occupied("")
	cands := make([][][]Seat, len(reqs))
	costs := make([][]int, len(reqs))
	for i, r := range reqs {
		cands[i] = h.candidates(r.Size, r.Pref, occ)
		if len(cands[i]) == 0 {
			return nil, fmt.Errorf("order %q: no contiguous run of %d available", r.ID, r.Size)
		}
		costs[i] = make([]int, len(cands[i]))
		for k, run := range cands[i] {
			costs[i][k] = runCost(run, r.Pref)
		}
	}
	choice, _, ok := search(cands, costs)
	if !ok {
		return nil, fmt.Errorf("no feasible joint assignment for batch")
	}
	out := make([]*Order, len(reqs))
	for i, r := range reqs {
		o := &Order{ID: r.ID, Size: r.Size, Pref: r.Pref, Seats: choice[i], Cost: runCost(choice[i], r.Pref)}
		h.Orders[r.ID] = o
		out[i] = o
	}
	return out, nil
}

// Change reseats one order with a new size and preference. The order may
// reuse its own seats; other orders are never moved. The new assignment is
// computed fully before replacing the old one, so failure leaves the order
// untouched.
func (h *Hall) Change(id string, size int, pref Seat) (*Order, error) {
	o := h.Orders[id]
	if o == nil {
		return nil, fmt.Errorf("order %q not found", id)
	}
	if size < 1 {
		return nil, fmt.Errorf("size must be positive")
	}
	occ := h.occupied(id)
	cands := h.candidates(size, pref, occ)
	if len(cands) == 0 {
		return nil, fmt.Errorf("order %q: no contiguous run of %d available", id, size)
	}
	costs := make([]int, len(cands))
	for k, run := range cands {
		costs[k] = runCost(run, pref)
	}
	choice, _, ok := search([][][]Seat{cands}, [][]int{costs})
	if !ok {
		return nil, fmt.Errorf("order %q: no feasible assignment", id)
	}
	o.Size = size
	o.Pref = pref
	o.Seats = choice[0]
	o.Cost = runCost(choice[0], pref)
	return o, nil
}

// Release removes an order and frees its seats.
func (h *Hall) Release(id string) (*Order, error) {
	o := h.Orders[id]
	if o == nil {
		return nil, fmt.Errorf("order %q not found", id)
	}
	delete(h.Orders, id)
	return o, nil
}
