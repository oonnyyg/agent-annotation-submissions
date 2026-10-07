package dispense

// Solve finds a combination of denominations that sums exactly to amount,
// using at most avail[i] notes of denoms[i]. denoms must be sorted in
// descending order and contain only positive values.
//
// The chosen combination minimizes the total note count; ties are broken by
// preferring more notes of larger denominations (usage vectors are compared
// lexicographically from the largest denomination downward). Returns nil if
// no exact combination exists.
func Solve(amount int64, denoms []int64, avail []int64) []int64 {
	if amount < 0 || len(denoms) == 0 {
		return nil
	}
	if amount == 0 {
		return make([]int64, len(denoms))
	}
	// best[s] holds the optimal usage vector for sum s, nil if unreachable.
	best := make([][]int64, amount+1)
	best[0] = make([]int64, len(denoms))
	for s := int64(1); s <= amount; s++ {
		var cand []int64
		for i, d := range denoms {
			if d <= 0 || d > s {
				continue
			}
			prev := best[s-d]
			if prev == nil || prev[i] >= avail[i] {
				continue
			}
			next := make([]int64, len(denoms))
			copy(next, prev)
			next[i]++
			if cand == nil || better(next, cand) {
				cand = next
			}
		}
		best[s] = cand
	}
	return best[amount]
}

// better reports whether usage vector a is preferred over b: fewer total
// notes, or equal notes with more usage of larger denominations first.
func better(a, b []int64) bool {
	var ta, tb int64
	for i := range a {
		ta += a[i]
		tb += b[i]
	}
	if ta != tb {
		return ta < tb
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
