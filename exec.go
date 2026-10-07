package rename

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options controls execution, including test hooks for failure injection.
type Options struct {
	// FailBeforeStep, when >= 0, injects a failure right before the rename
	// step with that index (steps are ordered: all to-temp moves first,
	// then all temp-to-final moves).
	FailBeforeStep int
	// FailRollbackStep, when >= 0, injects a failure into the rollback step
	// with that index (rollback runs in reverse completion order).
	FailRollbackStep int
}

// StepLog describes one completed rename step.
type StepLog struct {
	Index int    `json:"index"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// FinalLocation reports where the content of one source file ended up.
type FinalLocation struct {
	Old    string `json:"old"`
	Where  string `json:"where"`
	SHA256 string `json:"sha256"`
}

// Result is the execution report.
type Result struct {
	Steps      []StepLog       `json:"steps"`
	Final      []FinalLocation `json:"final"`
	RolledBack bool            `json:"rolledBack"`
	// RollbackErrors lists rollback steps that failed; when non-empty the
	// batch is NOT fully restored and Final reports actual locations.
	RollbackErrors []string `json:"rollbackErrors,omitempty"`
}

type completedMove struct {
	from, to string // base names
}

func tempName(e Entry, i int) string {
	return fmt.Sprintf(".tbr-%d-%s.tmp", i, e.SHA256[:16])
}

// verify rechecks, right before execution, that sources still match the
// preview hashes and that no new directory conflicts appeared. Any drift
// rejects the whole batch.
func verify(plan *Plan) error {
	srcSet := map[string]bool{}
	for _, e := range plan.Entries {
		srcSet[strings.ToLower(e.Old)] = true
	}
	for _, e := range plan.Entries {
		sum, err := hashFile(filepath.Join(plan.Dir, e.Old))
		if err != nil {
			return fmt.Errorf("source %q changed since preview: %w", e.Old, err)
		}
		if sum != e.SHA256 {
			return fmt.Errorf("source %q content changed since preview; refusing to execute", e.Old)
		}
	}
	for _, e := range plan.Entries {
		tp := filepath.Join(plan.Dir, e.New)
		if _, err := os.Lstat(tp); err == nil {
			if !srcSet[strings.ToLower(e.New)] {
				return fmt.Errorf("target %q now exists and is not a batch source; refusing to execute", e.New)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Execute runs the plan transactionally. On any step failure it rolls back
// completed steps in reverse order. It never overwrites an unrelated file:
// every rename target is either a batch temp name or a validated target.
func Execute(plan *Plan, opts Options) (*Result, error) {
	if err := verify(plan); err != nil {
		return nil, err
	}
	res := &Result{}
	var done []completedMove

	// Full step sequence: phase 1 (old -> temp), phase 2 (temp -> new).
	type step struct{ from, to string }
	var steps []step
	for i, e := range plan.Entries {
		steps = append(steps, step{e.Old, tempName(e, i)})
	}
	for i, e := range plan.Entries {
		steps = append(steps, step{tempName(e, i), e.New})
	}

	fail := func(err error) (*Result, error) {
		res.RolledBack = true
		rollback(plan, done, res, opts)
		fillFinal(plan, done, res, false)
		return res, err
	}

	for idx, s := range steps {
		if opts.FailBeforeStep >= 0 && idx == opts.FailBeforeStep {
			return fail(fmt.Errorf("injected failure before step %d (%s -> %s)", idx, s.from, s.to))
		}
		// Never clobber an unrelated file: target must not exist unless it
		// is a batch source still awaiting its own move.
		if _, err := os.Lstat(filepath.Join(plan.Dir, s.to)); err == nil {
			isBatchSrc := false
			for _, e := range plan.Entries {
				if strings.EqualFold(e.Old, s.to) {
					isBatchSrc = true
					break
				}
			}
			if !isBatchSrc {
				return fail(fmt.Errorf("step %d: target %q unexpectedly exists", idx, s.to))
			}
		}
		if err := os.Rename(filepath.Join(plan.Dir, s.from), filepath.Join(plan.Dir, s.to)); err != nil {
			return fail(fmt.Errorf("step %d (%s -> %s): %w", idx, s.from, s.to, err))
		}
		res.Steps = append(res.Steps, StepLog{Index: idx, From: s.from, To: s.to})
		done = append(done, completedMove{from: s.from, to: s.to})
	}
	fillFinal(plan, done, res, true)
	return res, nil
}

// rollback undoes completed moves in reverse order. A failed rollback step
// is recorded and the file is left where it still has content; nothing is
// ever deleted.
func rollback(plan *Plan, done []completedMove, res *Result, opts Options) {
	rbIdx := 0
	for i := len(done) - 1; i >= 0; i-- {
		mv := done[i]
		if opts.FailRollbackStep >= 0 && rbIdx == opts.FailRollbackStep {
			res.RollbackErrors = append(res.RollbackErrors,
				fmt.Sprintf("injected rollback failure at step %d (%s -> %s)", rbIdx, mv.to, mv.from))
			rbIdx++
			continue
		}
		dst := filepath.Join(plan.Dir, mv.from)
		if _, err := os.Lstat(dst); err == nil {
			res.RollbackErrors = append(res.RollbackErrors,
				fmt.Sprintf("cannot restore %q: name occupied; content remains at %q", mv.from, mv.to))
			rbIdx++
			continue
		}
		if err := os.Rename(filepath.Join(plan.Dir, mv.to), dst); err != nil {
			res.RollbackErrors = append(res.RollbackErrors,
				fmt.Sprintf("restore %q -> %q failed: %v; content remains at %q", mv.to, mv.from, err, mv.to))
		}
		rbIdx++
	}
}

// fillFinal computes where each source's content actually resides by
// tracking content hashes through the completed moves, and (on success)
// verifies no batch temp files remain.
func fillFinal(plan *Plan, done []completedMove, res *Result, success bool) {
	hashAt := map[string]string{} // current name -> original content hash
	for _, e := range plan.Entries {
		hashAt[e.Old] = e.SHA256
	}
	for _, mv := range done {
		if h, ok := hashAt[mv.from]; ok {
			delete(hashAt, mv.from)
			hashAt[mv.to] = h
		}
	}
	origByHash := map[string]string{}
	for _, e := range plan.Entries {
		origByHash[e.SHA256] = e.Old
	}
	for name, h := range hashAt {
		res.Final = append(res.Final, FinalLocation{Old: origByHash[h], Where: name, SHA256: h})
	}
	if success {
		for i, e := range plan.Entries {
			p := filepath.Join(plan.Dir, tempName(e, i))
			if _, err := os.Lstat(p); err == nil {
				res.RollbackErrors = append(res.RollbackErrors,
					fmt.Sprintf("leftover temp file %q after success", tempName(e, i)))
			} else if !os.IsNotExist(err) {
				res.RollbackErrors = append(res.RollbackErrors, err.Error())
			}
		}
	}
}
