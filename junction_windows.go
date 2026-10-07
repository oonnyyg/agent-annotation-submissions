//go:build windows

package main

import (
	"os"
	"syscall"
)

// isJunction reports whether the directory is a Windows reparse point
// (junction or mount point). These are never followed.
func isJunction(info os.FileInfo) bool {
	if !info.IsDir() {
		return false
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
