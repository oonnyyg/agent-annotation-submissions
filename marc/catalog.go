package marc

import "strings"

// Entry is the bibliographic view of one record.
type Entry struct {
	Index         int      // 0-based record sequence among valid records
	ControlNumber string   // 001 control field
	Title         string   // all 245 $a/$b joined with a space, in directory order
	ISBNs         []string // valid normalized ISBN-13s, in record order
	InvalidISBNs  []string // raw 020 $a values that failed validation
}

// ExtractEntry pulls the control number, title and ISBNs from a record.
// Repeated fields and subfields keep their original order.
func ExtractEntry(rec *Record, index int) Entry {
	e := Entry{Index: index}
	var titleParts []string
	for _, f := range rec.Fields {
		switch f.Tag {
		case "001":
			if e.ControlNumber == "" {
				e.ControlNumber = strings.TrimSpace(f.Value)
			}
		case "020":
			for _, sf := range f.Subfields {
				if sf.Code != 'a' {
					continue
				}
				if norm, ok := NormalizeISBN(sf.Data); ok {
					e.ISBNs = append(e.ISBNs, norm)
				} else {
					e.InvalidISBNs = append(e.InvalidISBNs, sf.Data)
				}
			}
		case "245":
			for _, sf := range f.Subfields {
				if sf.Code == 'a' || sf.Code == 'b' {
					titleParts = append(titleParts, sf.Data)
				}
			}
		}
	}
	e.Title = strings.Join(titleParts, " ")
	return e
}

// Group is a connected component of records sharing valid ISBNs.
type Group struct {
	ID       int
	Members  []int    // entry indexes
	ISBNs    []string // union of normalized ISBNs in the group
	Conflict bool     // titles differ after whitespace collapsing
}

// collapseWhitespace compares titles after compressing whitespace runs.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// BuildGroups unions entries by shared valid ISBNs and flags groups whose
// member titles differ (compared after whitespace collapsing only; no
// automatic merging).
func BuildGroups(entries []Entry) []Group {
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
		for _, isbn := range e.ISBNs {
			if j, seen := byISBN[isbn]; seen {
				union(i, j)
			} else {
				byISBN[isbn] = i
			}
		}
	}
	order := []int{}
	members := map[int][]int{}
	for i := range entries {
		root := find(i)
		if _, seen := members[root]; !seen {
			order = append(order, root)
		}
		members[root] = append(members[root], i)
	}
	groups := make([]Group, 0, len(order))
	for id, root := range order {
		g := Group{ID: id, Members: members[root]}
		isbnSeen := map[string]bool{}
		titleSeen := map[string]bool{}
		for _, m := range g.Members {
			for _, isbn := range entries[m].ISBNs {
				if !isbnSeen[isbn] {
					isbnSeen[isbn] = true
					g.ISBNs = append(g.ISBNs, isbn)
				}
			}
			titleSeen[collapseWhitespace(entries[m].Title)] = true
		}
		g.Conflict = len(titleSeen) > 1
		groups = append(groups, g)
	}
	return groups
}
