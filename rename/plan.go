// Package rename implements a transactional batch rename tool:
// preview a JSON plan first, then execute it with verification,
// two-phase temp-name renaming, and best-effort rollback.
package rename

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one rename operation in a plan.
type Entry struct {
	Old    string `json:"old"`    // original base name
	New    string `json:"new"`    // target base name
	SHA256 string `json:"sha256"` // content digest of the source at preview time
}

// Plan is a serializable rename plan for one directory.
type Plan struct {
	Dir     string  `json:"dir"`
	Entries []Entry `json:"entries"`
}

// validBaseName rejects path separators and parent references.
func validBaseName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid name %q", name)
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') ||
		filepath.Base(name) != name {
		return fmt.Errorf("name %q must be a plain base name without path separators", name)
	}
	return nil
}

// hashFile returns the SHA-256 hex digest of a regular file, refusing symlinks.
func hashFile(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BuildPlan previews a rename batch: it validates the mapping, hashes the
// current sources and returns a Plan. No files are modified.
func BuildPlan(dir string, pairs [][2]string) (*Plan, error) {
	if len(pairs) == 0 {
		return nil, errors.New("empty mapping")
	}
	plan := &Plan{Dir: dir}
	srcSeen := map[string]string{} // lower-case base name -> original
	dstSeen := map[string]string{}
	for _, p := range pairs {
		old, newName := p[0], p[1]
		if err := validBaseName(old); err != nil {
			return nil, fmt.Errorf("source: %w", err)
		}
		if err := validBaseName(newName); err != nil {
			return nil, fmt.Errorf("target: %w", err)
		}
		lo, ln := strings.ToLower(old), strings.ToLower(newName)
		if prev, dup := srcSeen[lo]; dup {
			return nil, fmt.Errorf("duplicate source %q and %q (case-insensitive)", prev, old)
		}
		if prev, dup := dstSeen[ln]; dup {
			return nil, fmt.Errorf("duplicate target %q and %q (case-insensitive)", prev, newName)
		}
		srcSeen[lo] = old
		dstSeen[ln] = newName

		full := filepath.Join(dir, old)
		fi, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", old, err)
		}
		if !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("source %q is not a regular file (links are not followed)", old)
		}
		sum, err := hashFile(full)
		if err != nil {
			return nil, err
		}
		plan.Entries = append(plan.Entries, Entry{Old: old, New: newName, SHA256: sum})
	}
	// Targets may exist only if they are themselves sources of this batch.
	for _, e := range plan.Entries {
		if _, isSrc := srcSeen[strings.ToLower(e.New)]; isSrc {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dir, e.New)); err == nil {
			return nil, fmt.Errorf("target %q already exists and is not part of this batch", e.New)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return plan, nil
}

// SavePlan writes the plan as indented JSON.
func SavePlan(plan *Plan, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(plan)
}

// LoadPlan reads and validates a plan file.
func LoadPlan(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("invalid plan file: %w", err)
	}
	if plan.Dir == "" || len(plan.Entries) == 0 {
		return nil, errors.New("plan has no directory or entries")
	}
	for _, e := range plan.Entries {
		if err := validBaseName(e.Old); err != nil {
			return nil, fmt.Errorf("plan source: %w", err)
		}
		if err := validBaseName(e.New); err != nil {
			return nil, fmt.Errorf("plan target: %w", err)
		}
	}
	return &plan, nil
}

// Verify rechecks, at execution time, that sources still match the previewed
// content and that no new directory conflicts appeared. Any drift rejects the
// whole batch; the plan is never silently regenerated.
func Verify(plan *Plan) error {
	srcSet := map[string]bool{}
	for _, e := range plan.Entries {
		srcSet[strings.ToLower(e.Old)] = true
		sum, err := hashFile(filepath.Join(plan.Dir, e.Old))
		if err != nil {
			return fmt.Errorf("source %q changed since preview: %v", e.Old, err)
		}
		if sum != e.SHA256 {
			return fmt.Errorf("source %q content changed since preview; refusing to execute", e.Old)
		}
	}
	for _, e := range plan.Entries {
		if srcSet[strings.ToLower(e.New)] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(plan.Dir, e.New)); err == nil {
			return fmt.Errorf("target %q now exists and is not part of this batch", e.New)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Summary lists final locations after a successful run.
func (plan *Plan) Summary() []string {
	var out []string
	for _, e := range plan.Entries {
		out = append(out, fmt.Sprintf("%s -> %s", e.Old, e.New))
	}
	sort.Strings(out)
	return out
}
