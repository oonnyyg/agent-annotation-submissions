package main

import (
	"fmt"
	"sort"
	"strings"
)

// Config holds the anonymization parameters. All thresholds are positive
// integers; Budget is a non-negative integer.
type Config struct {
	MinGroupSize    int   // k: minimum records per equivalence group
	MinDistinctSens int   // l: minimum distinct sensitive values per group
	Budget          int   // maximum total suppressed rows
	Weights         []int // positive cost weight per quasi-identifier column
	SuppressCost    int   // positive cost per suppressed record
}

func (c *Config) Validate(nQI int) error {
	if c.MinGroupSize < 1 {
		return fmt.Errorf("min group size must be a positive integer")
	}
	if c.MinDistinctSens < 1 {
		return fmt.Errorf("min distinct sensitive values must be a positive integer")
	}
	if c.Budget < 0 {
		return fmt.Errorf("suppression budget must be a non-negative integer")
	}
	if len(c.Weights) != nQI {
		return fmt.Errorf("expected %d weights, got %d", nQI, len(c.Weights))
	}
	for i, w := range c.Weights {
		if w < 1 {
			return fmt.Errorf("weight for column %d must be a positive integer", i+1)
		}
	}
	if c.SuppressCost < 1 {
		return fmt.Errorf("suppression cost must be a positive integer")
	}
	return nil
}

// GroupReport describes one equivalence group after generalization.
type GroupReport struct {
	Key          []string `json:"key"`
	Size         int      `json:"size"`
	DistinctSens int      `json:"distinctSensitive"`
	OK           bool     `json:"ok"`
	Suppressed   bool     `json:"suppressed"`
	RecordIDs    []string `json:"recordIds"`
}

// RowReport records the keep/drop decision for one input record.
type RowReport struct {
	ID          string   `json:"id"`
	Kept        bool     `json:"kept"`
	GroupKey    []string `json:"groupKey"`
	GroupOK     bool     `json:"groupOk"`
	Generalized []string `json:"generalized"`
}

// Evaluation is the result of applying one step vector.
type Evaluation struct {
	Steps      []int
	Suppressed int
	Cost       int
	Feasible   bool
	FailReason string
	Groups     []GroupReport
	Rows       []RowReport
	Kept       []KeptRecord
}

// KeptRecord is one published row.
type KeptRecord struct {
	QI        []string
	Sensitive string
}

// Evaluate applies the step vector to the dataset and checks all constraints.
func Evaluate(ds *Dataset, hs map[string]*Hierarchy, cfg *Config, steps []int) (*Evaluation, error) {
	if len(steps) != len(ds.QIColumns) {
		return nil, fmt.Errorf("expected %d steps, got %d", len(ds.QIColumns), len(steps))
	}
	ev := &Evaluation{Steps: append([]int(nil), steps...)}
	gen := make([][]string, len(ds.Records))
	for i, rec := range ds.Records {
		g := make([]string, len(ds.QIColumns))
		for j, col := range ds.QIColumns {
			v, err := hs[col].Generalize(rec.QI[j], steps[j])
			if err != nil {
				return nil, fmt.Errorf("record %q: %w", rec.ID, err)
			}
			g[j] = v
		}
		gen[i] = g
	}
	// Group by generalized key; iterate groups in sorted key order so the
	// result is independent of input row order.
	type grp struct {
		idxs []int
		sens map[string]bool
	}
	groups := map[string]*grp{}
	for i, g := range gen {
		key := strings.Join(g, "\x00")
		gr, ok := groups[key]
		if !ok {
			gr = &grp{sens: map[string]bool{}}
			groups[key] = gr
		}
		gr.idxs = append(gr.idxs, i)
		gr.sens[ds.Records[i].Sensitive] = true
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	suppressed := 0
	for _, key := range keys {
		gr := groups[key]
		gk := gen[gr.idxs[0]]
		rep := GroupReport{Key: gk, Size: len(gr.idxs), DistinctSens: len(gr.sens)}
		rep.OK = rep.Size >= cfg.MinGroupSize && rep.DistinctSens >= cfg.MinDistinctSens
		rep.Suppressed = !rep.OK
		if rep.Suppressed {
			suppressed += rep.Size
		}
		ids := make([]string, 0, len(gr.idxs))
		for _, i := range gr.idxs {
			ids = append(ids, ds.Records[i].ID)
		}
		sort.Strings(ids)
		rep.RecordIDs = ids
		ev.Groups = append(ev.Groups, rep)
		for _, i := range gr.idxs {
			ev.Rows = append(ev.Rows, RowReport{
				ID:          ds.Records[i].ID,
				Kept:        rep.OK,
				GroupKey:    gk,
				GroupOK:     rep.OK,
				Generalized: gen[i],
			})
			if rep.OK {
				ev.Kept = append(ev.Kept, KeptRecord{QI: gen[i], Sensitive: ds.Records[i].Sensitive})
			}
		}
	}
	sort.Slice(ev.Rows, func(a, b int) bool { return ev.Rows[a].ID < ev.Rows[b].ID })
	sort.Slice(ev.Kept, func(a, b int) bool {
		ka, kb := strings.Join(ev.Kept[a].QI, "\x00"), strings.Join(ev.Kept[b].QI, "\x00")
		if ka != kb {
			return ka < kb
		}
		return ev.Kept[a].Sensitive < ev.Kept[b].Sensitive
	})
	ev.Suppressed = suppressed
	cost := suppressed * cfg.SuppressCost
	for j, s := range steps {
		cost += s * cfg.Weights[j]
	}
	ev.Cost = cost
	switch {
	case suppressed > cfg.Budget:
		ev.FailReason = fmt.Sprintf("suppression %d exceeds budget %d", suppressed, cfg.Budget)
	case len(ev.Kept) == 0:
		ev.FailReason = "no records would remain"
	default:
		ev.Feasible = true
	}
	return ev, nil
}

// Search explores every allowed step vector and returns the optimal feasible
// one. Ordering: lowest cost, then fewer suppressed rows, then
// lexicographically smallest step vector in configured column order.
func Search(ds *Dataset, hs map[string]*Hierarchy, cfg *Config) (*Evaluation, error) {
	n := len(ds.QIColumns)
	maxSteps := make([]int, n)
	for j, col := range ds.QIColumns {
		maxSteps[j] = hs[col].MaxSteps()
	}
	var best *Evaluation
	steps := make([]int, n)
	var rec func(j int) error
	rec = func(j int) error {
		if j == n {
			ev, err := Evaluate(ds, hs, cfg, steps)
			if err != nil {
				return err
			}
			if !ev.Feasible {
				return nil
			}
			if best == nil || better(ev, best) {
				best = ev
			}
			return nil
		}
		for s := 0; s <= maxSteps[j]; s++ {
			steps[j] = s
			if err := rec(j + 1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := rec(0); err != nil {
		return nil, err
	}
	if best == nil {
		return nil, fmt.Errorf("no feasible generalization scheme satisfies the constraints within the suppression budget")
	}
	return best, nil
}

func better(a, b *Evaluation) bool {
	if a.Cost != b.Cost {
		return a.Cost < b.Cost
	}
	if a.Suppressed != b.Suppressed {
		return a.Suppressed < b.Suppressed
	}
	for j := range a.Steps {
		if a.Steps[j] != b.Steps[j] {
			return a.Steps[j] < b.Steps[j]
		}
	}
	return false
}
