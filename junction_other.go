//go:build !windows

package main

import "os"

// isJunction is always false on non-Windows platforms; directory
// junctions are a Windows concept. Symlinks are handled separately.
func isJunction(info os.FileInfo) bool {
	return false
}
