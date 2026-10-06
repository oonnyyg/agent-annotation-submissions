package torrent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ValidatePaths checks every declared path component for safety and
// uniqueness. It must pass before any content file is opened.
func ValidatePaths(m *Meta) error {
	seen := map[string]string{}
	for i, f := range m.Files {
		if len(f.Path) == 0 {
			return fmt.Errorf("torrent: files[%d] has an empty path", i)
		}
		for _, c := range f.Path {
			if err := checkComponent(c); err != nil {
				return fmt.Errorf("torrent: files[%d] (%s): %v", i, DisplayPath(f.Path), err)
			}
		}
		key := DisplayPath(f.Path)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("torrent: path %q conflicts with %q (case-insensitive on Windows)",
				DisplayPath(f.Path), prev)
		}
		seen[key] = DisplayPath(f.Path)
	}
	return nil
}

func checkComponent(c string) error {
	if c == "" {
		return fmt.Errorf("empty path component")
	}
	if c == "." || c == ".." {
		return fmt.Errorf("path component %q is not allowed", c)
	}
	if strings.ContainsAny(c, "/\\") {
		return fmt.Errorf("path component %q contains a separator", c)
	}
	if strings.ContainsRune(c, 0) {
		return fmt.Errorf("path component contains NUL")
	}
	if len(c) >= 2 && c[1] == ':' {
		return fmt.Errorf("path component %q looks like a drive absolute path", c)
	}
	return nil
}

// SafeJoin joins components under root and verifies the result stays inside
// root, resolving any symlinks in existing ancestors. It never follows a
// symlink at the final component for reading decisions; callers open the
// returned path only after this check passes.
func SafeJoin(root string, comps []string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}
	parts := append([]string{absRoot}, comps...)
	target := filepath.Join(parts...)

	// Resolve symlinks among existing ancestors of the target.
	dir := target
	var tail []string
	for {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		tail = append([]string{filepath.Base(dir)}, tail...)
		dir = parent
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	full := dir
	for _, t := range tail {
		full = filepath.Join(full, t)
	}
	if !withinRoot(absRoot, full) {
		return "", fmt.Errorf("path %q escapes the content root via a link", DisplayPath(comps))
	}
	return target, nil
}

func withinRoot(root, path string) bool {
	r := root
	p := path
	if runtime.GOOS == "windows" {
		r = strings.ToLower(r)
		p = strings.ToLower(p)
	}
	if p == r {
		return true
	}
	return strings.HasPrefix(p, r+string(os.PathSeparator))
}
