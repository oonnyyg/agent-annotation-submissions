package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/marc-isbn-catalog/marc"
)

// TestEndToEnd builds a small MARC file with a conflict group and a bad
// record, runs the pipeline, and checks CSV/JSON outputs.
func TestEndToEnd(t *testing.T) {
	rec := func(ctrl, title, isbn string) []byte {
		return testRecord(t, ctrl, title, isbn)
	}
	good1 := rec("c1", "题名甲", "0-306-40615-2")
	good2 := rec("c2", "题名乙", "9780306406157")
	bad := testRecord(t, "c3", "坏记录", "9787020002207")
	bad[len(bad)-2] = 'X'
	good3 := rec("c4", "独立", "979-10-90636-07-1")
	var data []byte
	data = append(append(append(append(data, good1...), bad...), good2...), good3...)

	dir := t.TempDir()
	mrc := filepath.Join(dir, "in.mrc")
	if err := os.WriteFile(mrc, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(mrc)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, badRecs, stopErr := marc.ReadAll(f)
	if stopErr != nil || len(records) != 3 || len(badRecs) != 1 {
		t.Fatalf("records=%d bad=%d stop=%v", len(records), len(badRecs), stopErr)
	}
	entries := make([]marc.Entry, len(records))
	for i, r := range records {
		entries[i] = marc.ExtractEntry(r, i)
	}
	groups := marc.BuildGroups(entries)
	groupOf := map[int]int{}
	for _, g := range groups {
		for _, m := range g.Members {
			groupOf[m] = g.ID
		}
	}
	csvPath := filepath.Join(dir, "out.csv")
	jsonPath := filepath.Join(dir, "out.json")
	if err := writeCSV(csvPath, entries, groups, groupOf); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(jsonPath, entries, badRecs, groups, stopErr); err != nil {
		t.Fatal(err)
	}
	csvData, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(csvData)
	if !strings.Contains(s, "题名甲") || !strings.Contains(s, "true") {
		t.Errorf("csv missing title/conflict:\n%s", s)
	}
	js, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal(js, &d); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if d["records_parsed"].(float64) != 3 {
		t.Errorf("records_parsed = %v", d["records_parsed"])
	}
}

func testRecord(t *testing.T, ctrl, title, isbn string) []byte {
	t.Helper()
	fields := []struct{ tag, data string }{
		{"001", ctrl},
		{"245", "10\x1fa" + title},
		{"020", "  \x1fa" + isbn},
	}
	var dir, area strings.Builder
	base := 24 + len(fields)*12 + 1
	pos := 0
	for _, f := range fields {
		body := f.data + "\x1e"
		dir.WriteString(f.tag)
		dir.WriteString(pad(len(body), 4))
		dir.WriteString(pad(pos, 5))
		area.WriteString(body)
		pos += len(body)
	}
	recLen := base + pos + 1
	return []byte(pad(recLen, 5) + "nam a22" + pad(base, 5) + " a 4500" +
		dir.String() + "\x1e" + area.String() + "\x1d")
}

func pad(n, width int) string {
	s := strings.Repeat("0", width) + itoa(n)
	return s[len(s)-width:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
