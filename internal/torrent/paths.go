package torrent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePaths checks every declared file path and returns an error if any
// is unsafe: absolute paths, parent traversal, reserved components, or
// case-insensitive collisions between distinct paths.
func ValidatePaths(m *Meta) error {
	seen := map[string]string{}
	for _, f := range m.Files {
		if len(f.Path) == 0 {
			return fmt.Errorf("torrent: empty path for file")
		}
		for _, c := range f.Path {
			if err := checkComponent(c); err != nil {
				return fmt.Errorf("torrent: unsafe path %q: %v", strings.Join(f.Path, "/"), err)
			}
		}
		joined := filepath.Join(f.Path...)
		key := strings.ToLower(joined)
		if prev, ok := seen[key]; ok && prev != joined {
			return fmt.Errorf("torrent: case-insensitive path collision: %q and %q", prev, joined)
		}
		seen[key] = joined
	}
	return nil
}

func checkComponent(c string) error {
	if c == "" || c == "." || c == ".." {
		return fmt.Errorf("illegal path component %q", c)
	}
	if strings.ContainsAny(c, `/\`) {
		return fmt.Errorf("path component %q contains a separator", c)
	}
	if filepath.IsAbs(c) || len(c) >= 2 && c[1] == ':' {
		return fmt.Errorf("absolute or drive-relative component %q", c)
	}
	return nil
}

// SafeJoin resolves f's declared path under root and verifies the result —
// after resolving any existing symlinks — stays inside root. It returns the
// absolute path to open. The file itself need not exist yet.
func SafeJoin(root string, f File) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = r
	}
	target := filepath.Join(append([]string{absRoot}, f.Path...)...)
	if !within(absRoot, target) {
		return "", fmt.Errorf("path escapes content root: %s", strings.Join(f.Path, "/"))
	}
	// Resolve symlinks along the longest existing prefix of the target.
	if resolved, err := evalExisting(target); err == nil && !within(absRoot, resolved) {
		return "", fmt.Errorf("symlink escapes content root: %s", strings.Join(f.Path, "/"))
	}
	return target, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// evalExisting resolves symlinks for the longest existing ancestor of p and
// rejoins the non-existent tail.
func evalExisting(p string) (string, error) {
	missing := []string{}
	cur := p
	for {
		if _, err := os.Lstat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p, nil
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
}
