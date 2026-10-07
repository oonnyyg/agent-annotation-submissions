// Package rename implements a transactional batch rename tool:
// preview a plan as JSON, then execute it with verification and rollback.
package rename

import (
	"crypto/sha256"
	"encoding/hex"
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
	Old string `json:"old"`
	New string `json:"new"`
	// SHA256 is the hex digest of the source file content at preview time.
	SHA256 string `json:"sha256"`
}

// Plan is a serializable rename plan for one directory.
type Plan struct {
	Dir     string  `json:"dir"`
	Entries []Entry `json:"entries"`
}

// validBaseName rejects path separators, parent references, and anything
// that is not a plain file base name.
func validBaseName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid name %q", name)
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') ||
		strings.ContainsRune(name, os.PathSeparator) {
		return fmt.Errorf("name %q must not contain path separators", name)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("name %q must be a plain base name", name)
	}
	return nil
}

// hashFile returns the SHA-256 hex digest of a regular file, refusing
// anything that is not a regular file (e.g. symlinks).
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

// BuildPlan validates the requested mapping against the directory and
// returns a preview plan. It never modifies any file.
//
// pairs is a list of [old, new] base names. Rules:
//   - names must be plain base names (no separators, no "..");
//   - sources must exist as regular files (no symlink following);
//   - sources and targets must be unique case-insensitively;
//   - a target may already exist only if it is itself a source in this batch.
func BuildPlan(dir string, pairs [][2]string) (*Plan, error) {
	if len(pairs) == 0 {
		return nil, errors.New("empty rename batch")
	}
	di, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !di.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	srcSet := map[string]string{} // lower(old) -> old
	dstSet := map[string]string{} // lower(new) -> new
	for _, p := range pairs {
		old, newName := p[0], p[1]
		if err := validBaseName(old); err != nil {
			return nil, fmt.Errorf("source: %w", err)
		}
		if err := validBaseName(newName); err != nil {
			return nil, fmt.Errorf("target: %w", err)
		}
		lo, ln := strings.ToLower(old), strings.ToLower(newName)
		if prev, dup := srcSet[lo]; dup {
			return nil, fmt.Errorf("duplicate source %q and %q (case-insensitive)", prev, old)
		}
		if prev, dup := dstSet[ln]; dup {
			return nil, fmt.Errorf("duplicate target %q and %q (case-insensitive)", prev, newName)
		}
		srcSet[lo] = old
		dstSet[ln] = newName
	}

	plan := &Plan{Dir: dir}
	for _, p := range pairs {
		old, newName := p[0], p[1]
		full := filepath.Join(dir, old)
		sum, err := hashFile(full)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", old, err)
		}
		// Target conflict: allowed only if the existing entry is itself a
		// source of this batch.
		if _, err := os.Lstat(filepath.Join(dir, newName)); err == nil {
			if _, ok := srcSet[strings.ToLower(newName)]; !ok {
				return nil, fmt.Errorf("target %q already exists and is not part of this batch", newName)
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		plan.Entries = append(plan.Entries, Entry{Old: old, New: newName, SHA256: sum})
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].Old < plan.Entries[j].Old })
	return plan, nil
}
