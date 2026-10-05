package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Result is the per-target outcome reported in the JSON report.
type Result struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // cached, rebuilt, failed, skipped, pending-upstream
	Reason     string `json:"reason"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// Run executes (or, when planOnly, previews) the targets in dependency order.
func Run(m *Manifest, cache *Cache, root, logDir string, order []string, planOnly bool) []Result {
	results := map[string]*Result{}
	depHashes := map[string]map[string]string{} // target -> output hashes
	var out []Result
	for _, name := range order {
		t := m.Targets[name]
		r := &Result{Name: name}
		results[name] = r

		// Skip if any dependency failed or was skipped.
		failedDep := ""
		for _, d := range t.Deps {
			if dr := results[d]; dr != nil && (dr.Status == "failed" || dr.Status == "skipped") {
				failedDep = d
				break
			}
		}
		if failedDep != "" {
			r.Status = "skipped"
			r.Reason = fmt.Sprintf("dependency %q did not complete successfully", failedDep)
			out = append(out, *r)
			continue
		}

		base, err := Fingerprint(root, t)
		if err != nil {
			r.Status = "failed"
			r.Reason = "cannot read inputs: " + err.Error()
			out = append(out, *r)
			continue
		}
		// In plan mode, if a dependency will rebuild, downstream fingerprints
		// cannot be known yet.
		if planOnly {
			pending := false
			for _, d := range t.Deps {
				if results[d].Status == "rebuild" || results[d].Status == "pending-upstream" {
					pending = true
					break
				}
			}
			if pending {
				r.Status = "pending-upstream"
				r.Reason = "a dependency is scheduled to rebuild; new output fingerprint not yet known"
				out = append(out, *r)
				continue
			}
		}
		dh := map[string]string{}
		for _, d := range t.Deps {
			for f, h := range depHashes[d] {
				dh[d+":"+f] = h
			}
		}
		fp := fingerprintWithDeps(base, dh)
		hit, why := CheckHit(root, t, cache.Entries[name], fp)
		if hit {
			r.Status = "cached"
			r.Reason = "fingerprint and outputs unchanged"
			depHashes[name] = cache.Entries[name].OutputHashes
			out = append(out, *r)
			continue
		}
		if planOnly {
			r.Status = "rebuild"
			r.Reason = why
			out = append(out, *r)
			continue
		}

		start := time.Now()
		runErr := runCommand(root, logDir, t)
		r.DurationMs = time.Since(start).Milliseconds()
		if runErr != nil {
			r.Status = "failed"
			r.Reason = fmt.Sprintf("rebuild needed (%s); command failed: %v", why, runErr)
			out = append(out, *r)
			continue
		}
		hashes, err := HashOutputs(root, t)
		if err != nil {
			r.Status = "failed"
			r.Reason = fmt.Sprintf("rebuild needed (%s); %v", why, err)
			out = append(out, *r)
			continue
		}
		cache.Entries[name] = &CacheEntry{Fingerprint: fp, OutputHashes: hashes}
		depHashes[name] = hashes
		r.Status = "rebuilt"
		r.Reason = why
		out = append(out, *r)
	}
	return out
}

// runCommand starts the command directly from the argument array (no shell),
// with the work root as its fixed working directory, logging output per target.
func runCommand(root, logDir string, t *Target) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	logFile, err := os.Create(filepath.Join(logDir, t.Name+".log"))
	if err != nil {
		return err
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "$ workdir=%s args=%q\n", root, t.Command)
	cmd := exec.Command(t.Command[0], t.Command[1:]...)
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command exited with error: %w", err)
	}
	return nil
}
