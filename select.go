package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Selection describes a set of messages chosen by UID or by current
// sequence number. Items are "n" or closed ranges "a:b".
type Selection struct {
	By    string   `json:"by"` // "uid" or "seq"
	Items []string `json:"items"`
}

// parseNumberSet validates and expands items into a sorted, deduplicated
// set of positive numbers. Inverted ranges and non-positive or malformed
// numbers are rejected.
func parseNumberSet(items []string) ([]uint64, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("selection must contain at least one item")
	}
	set := map[uint64]bool{}
	add := func(tok string) (uint64, error) {
		n, err := strconv.ParseUint(tok, 10, 64)
		if err != nil || n == 0 {
			return 0, fmt.Errorf("invalid number %q: must be a positive integer", tok)
		}
		return n, nil
	}
	for _, item := range items {
		if lo, hi, ok := strings.Cut(item, ":"); ok {
			a, err := add(lo)
			if err != nil {
				return nil, err
			}
			b, err := add(hi)
			if err != nil {
				return nil, err
			}
			if a > b {
				return nil, fmt.Errorf("inverted range %q: start must not exceed end", item)
			}
			for n := a; n <= b; n++ {
				set[n] = true
			}
			continue
		}
		n, err := add(item)
		if err != nil {
			return nil, err
		}
		set[n] = true
	}
	nums := make([]uint64, 0, len(set))
	for n := range set {
		nums = append(nums, n)
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	return nums, nil
}

// resolveSelection maps a validated selection to message indexes in the
// mailbox. Numbers that do not exist are silently ignored. The full set is
// resolved before any mutation, so commands never chase moving sequence
// numbers mid-execution. Returned indexes are sorted ascending, which for
// "seq" selections preserves source order.
func resolveSelection(mb *Mailbox, sel *Selection) ([]int, error) {
	if sel.By != "uid" && sel.By != "seq" {
		return nil, fmt.Errorf("selection \"by\" must be \"uid\" or \"seq\", got %q", sel.By)
	}
	nums, err := parseNumberSet(sel.Items)
	if err != nil {
		return nil, err
	}
	var indexes []int
	if sel.By == "seq" {
		for _, n := range nums {
			if n <= uint64(len(mb.Messages)) {
				indexes = append(indexes, int(n)-1)
			}
		}
		return indexes, nil
	}
	want := map[uint64]bool{}
	for _, n := range nums {
		want[n] = true
	}
	for i, msg := range mb.Messages {
		if want[msg.UID] {
			indexes = append(indexes, i)
		}
	}
	return indexes, nil
}
