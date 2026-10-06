package auditor

import (
	"fmt"
	"path"
	"strings"
)

// Kind is the node type of a filesystem entry.
type Kind string

const (
	KindFile    Kind = "file"
	KindDir     Kind = "dir"
	KindSymlink Kind = "symlink"
)

// Entry is one node in the rebuilt filesystem view.
type Entry struct {
	Kind     Kind   `json:"kind"`
	SHA256   string `json:"sha256,omitempty"`
	Target   string `json:"target,omitempty"`
	Layer    int    `json:"layer"`
	Implicit bool   `json:"implicit,omitempty"`
}

// Event is one recorded change in a path's history.
type Event struct {
	Layer  int    `json:"layer"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// normalize validates and canonicalizes a tar member name.
func normalize(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("path %q contains backslash", name)
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("path %q is absolute", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes the root", name)
	}
	return clean, nil
}

// parentOf returns the parent directory of a normalized path, or "".
func parentOf(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return ""
	}
	return p[:i]
}

// isDescendantOf reports whether p lies strictly under dir.
func isDescendantOf(p, dir string) bool {
	return dir != "" && strings.HasPrefix(p, dir+"/")
}
