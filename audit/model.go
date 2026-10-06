// Package audit rebuilds a filesystem view from an ordered list of
// uncompressed tar layers, offline and without unpacking to disk.
//
// The model is intentionally restricted: only regular files, directories
// and symlinks are supported. Permissions, ownership and extended
// attributes are not audited.
package audit

import (
	"fmt"
	"path"
	"strings"
)

// Kind is the node type of a manifest entry.
type Kind string

const (
	KindFile    Kind = "file"
	KindDir     Kind = "dir"
	KindSymlink Kind = "symlink"
)

// Node is a single entry in the reconstructed filesystem view.
type Node struct {
	Kind   Kind   `json:"kind"`
	SHA256 string `json:"sha256,omitempty"` // regular files only
	Target string `json:"target,omitempty"` // symlinks only, recorded verbatim
	Layer  int    `json:"layer"`            // index of the layer that last wrote this node
}

// NormalizePath converts a tar member name into a canonical relative
// POSIX path. It rejects absolute paths, backslashes, empty results and
// any parent-directory traversal.
func NormalizePath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("path %q contains a backslash", name)
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("path %q is absolute", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", fmt.Errorf("path %q contains a parent-directory segment", name)
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes the layer root", name)
	}
	return clean, nil
}

// ParentOf returns the parent directory of a normalized path, or ""
// for top-level entries.
func ParentOf(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return ""
	}
	return p[:i]
}

// IsDescendantOf reports whether p lies strictly inside directory dir.
func IsDescendantOf(p, dir string) bool {
	return strings.HasPrefix(p, dir+"/")
}
