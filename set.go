package main

import "fmt"

// NumRange is a closed interval [Start, End]; a single number has
// Start == End.
type NumRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// Selection chooses messages by UID or by current sequence number.
type Selection struct {
	By     string     `json:"by"` // "uid" or "seq"
	Ranges []NumRange `json:"ranges"`
}

// validate checks the selection syntax before any resolution happens.
func (sel *Selection) validate() error {
	if sel.By != "uid" && sel.By != "seq" {
		return fmt.Errorf("selection \"by\" must be \"uid\" or \"seq\", got %q", sel.By)
	}
	if len(sel.Ranges) == 0 {
		return fmt.Errorf("selection must contain at least one range")
	}
	for _, r := range sel.Ranges {
		if r.Start < 1 {
			return fmt.Errorf("invalid number %d: numbering starts at 1", r.Start)
		}
		if r.End < r.Start {
			return fmt.Errorf("inverted range %d:%d", r.Start, r.End)
		}
	}
	return nil
}

// resolve maps the selection to sorted, deduplicated 0-based indexes
// into mb.Messages. Numbers that do not match any message are ignored.
// The whole selection is resolved before the caller mutates anything,
// so sequence numbers cannot shift mid-command.
func (sel *Selection) resolve(mb *Mailbox) ([]int, error) {
	if err := sel.validate(); err != nil {
		return nil, err
	}
	var indexes []int
	if sel.By == "seq" {
		// Clamp to the mailbox size so huge ranges terminate quickly.
		max := uint64(len(mb.Messages))
		for _, r := range sel.Ranges {
			end := r.End
			if end > max {
				end = max
			}
			for n := r.Start; n <= end; n++ {
				indexes = append(indexes, int(n-1))
			}
		}
	} else {
		uidToIndex := make(map[uint64]int, len(mb.Messages))
		var maxUID uint64
		for i, m := range mb.Messages {
			uidToIndex[m.UID] = i
			if m.UID > maxUID {
				maxUID = m.UID
			}
		}
		for _, r := range sel.Ranges {
			end := r.End
			if end > maxUID {
				end = maxUID
			}
			for n := r.Start; n <= end; n++ {
				if idx, ok := uidToIndex[n]; ok {
					indexes = append(indexes, idx)
				}
			}
		}
	}
	return sortedUniqueIndexes(indexes), nil
}
