package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CacheEntry records what a target was built from and what it produced.
type CacheEntry struct {
	Fingerprint  string            `json:"fingerprint"`
	OutputHashes map[string]string `json:"output_hashes"`
}

// Cache is a JSON-file-backed fingerprint store, reusable across runs.
type Cache struct {
	path    string
	Entries map[string]*CacheEntry `json:"entries"`
}

func LoadCache(dir string) (*Cache, error) {
	c := &Cache{path: filepath.Join(dir, "cache.json"), Entries: map[string]*CacheEntry{}}
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, fmt.Errorf("read cache: %w", err)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse cache: %w", err)
	}
	if c.Entries == nil {
		c.Entries = map[string]*CacheEntry{}
	}
	return c, nil
}

func (c *Cache) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func hashFile(root, rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Fingerprint computes a content fingerprint from the command arguments, the
// input file contents, and the output contents of direct dependencies.
func Fingerprint(root string, t *Target) (string, error) {
	h := sha256.New()
	h.Write([]byte("cmd\x00" + strings.Join(t.Command, "\x00")))
	inputs := append([]string{}, t.Inputs...)
	sort.Strings(inputs)
	for _, in := range inputs {
		sum, err := hashFile(root, in)
		if err != nil {
			return "", fmt.Errorf("input %q: %w", in, err)
		}
		h.Write([]byte("\x00in\x00" + filepath.Clean(in) + "\x00" + sum))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fingerprintWithDeps mixes direct dependency output hashes into the base fingerprint.
func fingerprintWithDeps(base string, depHashes map[string]string) string {
	h := sha256.New()
	h.Write([]byte(base))
	names := make([]string, 0, len(depHashes))
	for n := range depHashes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h.Write([]byte("\x00dep\x00" + n + "\x00" + depHashes[n]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CheckHit verifies a cache entry against the current fingerprint and the
// on-disk outputs. It returns whether the cache is usable and, if not, why.
func CheckHit(root string, t *Target, entry *CacheEntry, fingerprint string) (bool, string) {
	if entry == nil {
		return false, "no previous cache entry"
	}
	if entry.Fingerprint != fingerprint {
		return false, "fingerprint changed (command, inputs, or dependency outputs differ)"
	}
	for _, out := range t.Outputs {
		c := filepath.Clean(out)
		want, ok := entry.OutputHashes[c]
		if !ok {
			return false, fmt.Sprintf("output %q missing from cache entry", out)
		}
		got, err := hashFile(root, c)
		if err != nil {
			if os.IsNotExist(err) {
				return false, fmt.Sprintf("declared output %q is missing on disk", out)
			}
			return false, fmt.Sprintf("cannot read output %q: %v", out, err)
		}
		if got != want {
			return false, fmt.Sprintf("output %q was modified externally (content hash mismatch)", out)
		}
	}
	return true, ""
}

// HashOutputs records content hashes of the target's outputs after a build.
func HashOutputs(root string, t *Target) (map[string]string, error) {
	hashes := map[string]string{}
	for _, out := range t.Outputs {
		c := filepath.Clean(out)
		sum, err := hashFile(root, c)
		if err != nil {
			return nil, fmt.Errorf("declared output %q was not produced: %w", out, err)
		}
		hashes[c] = sum
	}
	return hashes, nil
}
