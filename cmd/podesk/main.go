// Command podesk merges an old PO catalog with a new POT template and
// emits an updated PO plus a release audit JSON.
//
// Usage: go run ./cmd/podesk -old old.po -pot new.pot -config config.json -out updated.po -audit audit.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/gettext-catalog-release/internal/catalog"
	"example.com/gettext-catalog-release/internal/po"
)

func main() {
	oldPath := flag.String("old", "", "path to old translated PO file (required)")
	potPath := flag.String("pot", "", "path to new POT template (required)")
	cfgPath := flag.String("config", "", "path to JSON config (required)")
	outPath := flag.String("out", "", "path for updated PO output (required)")
	auditPath := flag.String("audit", "", "path for audit JSON output (required)")
	flag.Parse()
	if *oldPath == "" || *potPath == "" || *cfgPath == "" || *outPath == "" || *auditPath == "" {
		fmt.Fprintln(os.Stderr, "all of -old, -pot, -config, -out, -audit are required")
		os.Exit(2)
	}

	readPO := func(path string) *po.File {
		data, err := os.ReadFile(path)
		if err != nil {
			fatal(err)
		}
		f, err := po.Parse(string(data))
		if err != nil {
			fatal(fmt.Errorf("%s: %w", path, err))
		}
		return f
	}
	oldCat := readPO(*oldPath)
	tmpl := readPO(*potPath)

	cfgData, err := os.ReadFile(*cfgPath)
	if err != nil {
		fatal(err)
	}
	var cfg catalog.Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		fatal(fmt.Errorf("config: %w", err))
	}
	if cfg.PluralSlots < 1 {
		fatal(fmt.Errorf("config: plural_slots must be >= 1"))
	}

	merged, err := catalog.Merge(oldCat, tmpl, cfg)
	if err != nil {
		fatal(err)
	}
	audit := catalog.Validate(merged, cfg)

	// Always write the PO for revision, even when issues exist.
	if err := os.WriteFile(*outPath, []byte(merged.String()), 0o644); err != nil {
		fatal(err)
	}
	auditJSON, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*auditPath, append(auditJSON, '\n'), 0o644); err != nil {
		fatal(err)
	}

	if !audit.ReleaseReady {
		fmt.Fprintf(os.Stderr, "release NOT ready: %d issue(s); see %s\n", len(audit.Issues), *auditPath)
		os.Exit(1)
	}
	fmt.Printf("release ready: %d/%d translated\n", audit.Stats.Translated, audit.Stats.Total)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "podesk:", err)
	os.Exit(2)
}