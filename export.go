package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manifest builds the human-readable checklist for the resolved objects.
func Manifest(objects []*Object) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s %-10s %-12s %-8s %-40s %s\n", "OFFSET", "TYPE", "SIZE", "DEPTH", "OBJECT-ID", "BASE")
	for _, o := range objects {
		base := o.BaseFrom
		if base == "" {
			base = "-"
		}
		fmt.Fprintf(&b, "%-10d %-10s %-12d %-8d %-40s %s\n",
			o.Entry.Offset, TypeName(o.Type), len(o.Data), o.Depth, o.ID, base)
	}
	return b.String()
}

// Export writes each object body to <dir>/<object-id> (raw bytes) plus a
// MANIFEST.txt checklist. The output is staged in a temporary sibling
// directory and only renamed into place after every object has been written
// successfully. An existing destination is never overwritten, and a failed
// export leaves no partial output directory behind.
func Export(dir string, objects []*Object) error {
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("output directory %s already exists", dir)
	} else if !os.IsNotExist(err) {
		return err
	}

	parent := filepath.Dir(dir)
	tmp, err := os.MkdirTemp(parent, ".pack-export-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	// Best-effort cleanup; after a successful rename tmp no longer exists.
	defer os.RemoveAll(tmp)

	for _, o := range objects {
		path := filepath.Join(tmp, o.ID)
		if err := os.WriteFile(path, o.Data, 0o644); err != nil {
			return fmt.Errorf("write object %s: %w", o.ID, err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "MANIFEST.txt"), []byte(Manifest(objects)), 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	if err := os.Rename(tmp, dir); err != nil {
		return fmt.Errorf("publish output directory: %w", err)
	}
	return nil
}
