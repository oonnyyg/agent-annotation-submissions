package main

import "sort"

// Plan maps denomination -> number of notes.
type Plan map[int]int

func (p Plan) total() int {
	sum := 0
	for d, n := range p {
		sum += d * n
	}
	return sum
}

func (p Plan) notes() int {
	sum := 0
	for _, n := range p {
		sum += n
	}
	return sum
}

// denomsDesc returns denominations sorted high to low.
func denomsDesc(avail Plan) []int {
	ds := make([]int, 0, len(avail))
	for d := range avail {
		ds = append(ds, d)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ds)))
	return ds
}

// MakePlan finds a combination of available notes summing exactly to amount.
// It minimizes total note count; among ties it prefers more larger
// denominations (comparing usage from highest denomination downward).
// Returns nil if amount cannot be composed from avail.
func MakePlan(amount int, avail Plan) Plan {
	if amount < 0 {
		return nil
	}
	if amount == 0 {
		return Plan{}
	}
	const inf = int(^uint(0) >> 2)
	ds := denomsDesc(avail)
	// Staged bounded knapsack: dp[i][a] = minimal note count to compose a
	// using denominations ds[i:] (ds sorted descending). Storing stages
	// makes reconstruction with limited counts straightforward.
	nd := len(ds)
	dp := make([][]int, nd+1)
	dp[nd] = make([]int, amount+1)
	for a := 1; a <= amount; a++ {
		dp[nd][a] = inf
	}
	for i := nd - 1; i >= 0; i-- {
		dp[i] = make([]int, amount+1)
		copy(dp[i], dp[i+1])
		d := ds[i]
		for n := 0; n < avail[d]; n++ {
			for a := amount; a >= d; a-- {
				if dp[i][a-d]+1 < dp[i][a] {
					dp[i][a] = dp[i][a-d] + 1
				}
			}
		}
	}
	if dp[0][amount] == inf {
		return nil
	}
	// Reconstruct walking stages from the largest denomination down,
	// taking as many of each as an optimal solution allows. This yields
	// the tie-break preference for larger denominations.
	plan := Plan{}
	a := amount
	for i := 0; i < nd && a > 0; i++ {
		d := ds[i]
		// Choose the maximal k notes of this denomination that still
		// allows an optimal completion with smaller denominations.
		for k := avail[d]; k >= 1; k-- {
			if k*d <= a && dp[i][a] == k+dp[i+1][a-k*d] {
				plan[d] = k
				a -= k * d
				break
			}
		}
	}
	if a != 0 {
		return nil // unreachable unless dp is inconsistent
	}
	return plan
}
