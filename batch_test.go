package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runErr(doc, path string, maxSize int64) error {
	segs, err := ParsePath(path)
	if err != nil {
		return err
	}
	_, err = Run(strings.NewReader(doc), segs, maxSize, func([]byte) error { return nil })
	return err
}

func TestRejectsDTD(t *testing.T) {
	doc := `<!DOCTYPE r [<!ENTITY x "y">]><r><rec>&x;</rec></r>`
	err := runErr(doc, "r/rec", 0)
	if err == nil || !strings.Contains(err.Error(), "DTD") {
		t.Fatalf("want DTD rejection, got %v", err)
	}
}

func TestUndeclaredPrefixPosition(t *testing.T) {
	doc := "<r>\n  <rec><q:item/></rec>\n</r>"
	err := runErr(doc, "r/rec", 0)
	if err == nil || !strings.Contains(err.Error(), `undeclared prefix "q"`) {
		t.Fatalf("want undeclared prefix error, got %v", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error should carry position, got %v", err)
	}
}

func TestTruncatedInput(t *testing.T) {
	err := runErr("<r><rec>partial", "r/rec", 0)
	if err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("want unexpected EOF, got %v", err)
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error should carry position, got %v", err)
	}
}

func TestRecordSizeLimit(t *testing.T) {
	doc := `<r><rec>` + strings.Repeat("x", 100) + `</rec></r>`
	err := runErr(doc, "r/rec", 64)
	if err == nil || !strings.Contains(err.Error(), "exceeds the maximum size") {
		t.Fatalf("want size limit failure, got %v", err)
	}
}

func TestTrailingErrorLeavesNoBatch(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "batch")
	doc := `<r><rec>ok</rec><rec>broken</rec>` // truncated tail
	segs, _ := ParsePath("r/rec")
	_, err := splitToDir(strings.NewReader(doc), out, segs, 0)
	if err == nil {
		t.Fatal("want error for truncated document")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("no batch directory may be published after a failure")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".xmlsplit-staging-") {
			t.Fatalf("staging directory %q was not cleaned up", e.Name())
		}
	}
}

func TestPublishBatchWithIndex(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "batch")
	doc := `<r><rec>a</rec><rec>b</rec></r>`
	segs, _ := ParsePath("r/rec")
	count, err := splitToDir(strings.NewReader(doc), out, segs, 0)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	data, err := os.ReadFile(filepath.Join(out, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx batchIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	if idx.Count != 2 || len(idx.Files) != 2 {
		t.Fatalf("bad index: %+v", idx)
	}
	for _, f := range idx.Files {
		fi, err := os.Stat(filepath.Join(out, f.Name))
		if err != nil || fi.Size() != f.Size || len(f.SHA256) != 64 {
			t.Fatalf("bad index entry %+v (stat err %v)", f, err)
		}
	}
	// Existing directory must not be overwritten.
	if _, err := splitToDir(strings.NewReader(doc), out, segs, 0); err == nil {
		t.Fatal("publishing over an existing directory must fail")
	}
}
