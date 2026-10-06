package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"example.com/gettext-catalog-release/internal/catalog"
	"example.com/gettext-catalog-release/internal/po"
)

func main() {
	oldPath := flag.String("old", "", "old PO file (input, never modified)")
	potPath := flag.String("pot", "", "new POT template file")
	cfgPath := flag.String("config", "", "JSON config with language and plural_slots")
	outPath := flag.String("out", "", "output PO file")
	auditPath := flag.String("audit", "", "output audit JSON file")
	flag.Parse()
	if *oldPath == "" || *potPath == "" || *cfgPath == "" || *outPath == "" || *auditPath == "" {
		fmt.Fprintln(os.Stderr, "usage: podesk -old old.po -pot new.pot -config cfg.json -out out.po -audit audit.json")
		os.Exit(2)
	}

	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "podesk:", err)
			os.Exit(2)
		}
		return string(b)
	}

	oldPO, err := po.Parse(read(*oldPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "podesk: old PO:", err)
		os.Exit(2)
	}
	pot, err := po.Parse(read(*potPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "podesk: POT:", err)
		os.Exit(2)
	}
	var cfg catalog.Config
	cfgText := strings.TrimPrefix(read(*cfgPath), "\ufeff")
	if err := json.Unmarshal([]byte(cfgText), &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "podesk: config:", err)
		os.Exit(2)
	}

	merged, err := catalog.Merge(oldPO, pot, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "podesk:", err)
		os.Exit(2)
	}
	audit := catalog.Review(merged, cfg)

	if err := os.WriteFile(*outPath, []byte(merged.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "podesk:", err)
		os.Exit(2)
	}
	aj, _ := json.MarshalIndent(audit, "", "  ")
	if err := os.WriteFile(*auditPath, append(aj, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "podesk:", err)
		os.Exit(2)
	}

	if !audit.ReleaseReady {
		fmt.Fprintf(os.Stderr, "podesk: %d issue(s); PO written for revision, release NOT approved\n", len(audit.Issues))
		os.Exit(1)
	}
	fmt.Println("podesk: release ready")
}
