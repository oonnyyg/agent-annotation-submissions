package catalog

import (
	"sort"
	"strings"
)

// Group is a connected component of records sharing valid canonical ISBNs.
type Group struct {
	ID       int      `json:"id"`
	ISBNs    []string `json:"isbns"`   // sorted canonical ISBN-13 values in the group
	Members  []int    `json:"members"` // entry indexes, ascending
	Titles   []string `json:"titles"`  // distinct whitespace-collapsed titles, first-seen order
	Conflict bool     `json:"conflict"`
}

// Groups links entries by shared valid ISBNs (union-find) and reports
// groups with more than one distinct collapsed title as conflicts.
// Invalid ISBNs never create links.
func Groups(entries []Entry) []Group {
	parent := make([]int, len(entries))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	byISBN := map[string]int{}
	for i, e := range entries {
		for _, canon := range e.ValidISBNs {
			if j, ok := byISBN[canon]; ok {
				union(i, j)
			} else {
				byISBN[canon] = i
			}
		}
	}

	order := []int{}
	members := map[int][]int{}
	for i := range entries {
		r := find(i)
		if _, ok := members[r]; !ok {
			order = append(order, r)
		}
		members[r] = append(members[r], i)
	}

	groups := make([]Group, 0, len(order))
	for _, r := range order {
		idxs := members[r]
		isbnSet := map[string]bool{}
		titleSet := map[string]bool{}
		g := Group{Members: idxs, ISBNs: []string{}}
		for _, i := range idxs {
			for _, canon := range entries[i].ValidISBNs {
				isbnSet[canon] = true
			}
			t := collapseSpace(entries[i].Title)
			if !titleSet[t] {
				titleSet[t] = true
				g.Titles = append(g.Titles, t)
			}
		}
		for canon := range isbnSet {
			g.ISBNs = append(g.ISBNs, canon)
		}
		sort.Strings(g.ISBNs)
		g.Conflict = len(g.Titles) > 1
		groups = append(groups, g)
	}
	for i := range groups {
		groups[i].ID = i + 1
	}
	return groups
}

// collapseSpace trims and compresses all whitespace runs to single spaces.
// Titles are compared only after this normalization; conflicting titles
// are reported, never merged.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
