package rename

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Step describes one completed rename, used for the execution log.
type Step struct {
	From string
	To   string
}

// Result reports the outcome of Execute.
type Result struct {
	Steps    []Step   // completed renames, in order
	Final    []string // final locations on success
	Rollback []Step   // rollback renames performed after a failure
	// Orphans lists files that still hold batch content but could not be
	// rolled back; they are preserved, never deleted.
	Orphans []string
}

// injectError is returned when FailBeforeStep triggers.
type injectError struct{ step int }

func (e injectError) Error() string {
	return fmt.Sprintf("injected failure before step %d", e.step)
}

// tempName creates a unique temp base name inside dir that collides with
// nothing, case-insensitively.
func tempName(dir string) (string, error) {
	var buf [8]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		name := ".tbr-tmp-" + hex.EncodeToString(buf[:])
		if _, err := os.Lstat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name, nil
		} else if err != nil {
			return "", err
		}
	}
}

// Execute runs a verified plan transactionally. Every source is first moved
// to an exclusive temp name (phase 1), then temps are moved to their final
// targets (phase 2), so swap cycles and case-only renames never overwrite
// unrelated files. If failBeforeStep > 0, an error is injected just before
// that 1-based rename step. On failure, completed steps are rolled back in
// reverse order; rollback failures preserve content and report locations.
func Execute(plan *Plan, failBeforeStep int) (*Result, error) {
	if err := Verify(plan); err != nil {
		return nil, err
	}
	res := &Result{}
	type op struct{ from, to string }
	var ops []op
	temps := make([]string, len(plan.Entries))
	for i, e := range plan.Entries {
		t, err := tempName(plan.Dir)
		if err != nil {
			return nil, err
		}
		temps[i] = t
		ops = append(ops, op{e.Old, t})
	}
	for i, e := range plan.Entries {
		ops = append(ops, op{temps[i], e.New})
	}

	rename := func(from, to string) error {
		return os.Rename(filepath.Join(plan.Dir, from), filepath.Join(plan.Dir, to))
	}

	for i, o := range ops {
		if failBeforeStep > 0 && i+1 == failBeforeStep {
			return rollback(plan, res, injectError{step: i + 1})
		}
		if err := rename(o.from, o.to); err != nil {
			return rollback(plan, res, fmt.Errorf("step %d (%s -> %s): %w", i+1, o.from, o.to, err))
		}
		res.Steps = append(res.Steps, Step{From: o.from, To: o.to})
	}
	res.Final = plan.Summary()
	return res, nil
}

// rollback undoes completed steps in reverse order. Temp files that no
// longer carry batch content are removed; anything still holding content is
// kept and reported as an orphan.
func rollback(plan *Plan, res *Result, cause error) (*Result, error) {
	var rbErrs []string
	for i := len(res.Steps) - 1; i >= 0; i-- {
		s := res.Steps[i]
		from := filepath.Join(plan.Dir, s.To)
		to := filepath.Join(plan.Dir, s.From)
		if err := os.Rename(from, to); err != nil {
			rbErrs = append(rbErrs, fmt.Sprintf("rollback %s -> %s failed: %v", s.To, s.From, err))
			if _, statErr := os.Lstat(from); statErr == nil {
				res.Orphans = append(res.Orphans, from)
			}
			continue
		}
		res.Rollback = append(res.Rollback, Step{From: s.To, To: s.From})
	}
	msg := cause.Error()
	if len(rbErrs) > 0 {
		msg += "; rollback incomplete: " + strings.Join(rbErrs, "; ")
	}
	return res, fmt.Errorf("batch failed, rolled back where possible: %s", msg)
}
