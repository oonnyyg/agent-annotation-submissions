package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const testHierarchy = `{
  "columns": {
    "age": {"value": "any", "children": [
      {"value": "young", "children": [{"value": "20"}, {"value": "25"}]},
      {"value": "old", "children": [{"value": "60"}, {"value": "65"}]}
    ]},
    "city": {"value": "cn", "children": [{"value": "bj"}, {"value": "sh"}]}
  }
}`

const testCSV = `id,age,city,disease
r1,20,bj,flu
r2,25,bj,cold
r3,60,sh,flu
r4,65,sh,cold
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadTest(t *testing.T, csvContent, hierContent string) (*Dataset, map[string]*Hierarchy) {
	t.Helper()
	ds, err := LoadDataset(writeTemp(t, "data.csv", csvContent))
	if err != nil {
		t.Fatal(err)
	}
	hs, err := LoadHierarchies(writeTemp(t, "h.json", hierContent))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLeaves(ds, hs); err != nil {
		t.Fatal(err)
	}
	return ds, hs
}

func testCfg() *Config {
	return &Config{MinGroupSize: 2, MinDistinctSens: 2, Budget: 4, Weights: []int{1, 1}, SuppressCost: 10}
}

func TestSearchFindsOptimal(t *testing.T) {
	ds, hs := loadTest(t, testCSV, testHierarchy)
	best, err := Search(ds, hs, testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(best.Steps, []int{1, 0}) {
		t.Fatalf("expected steps [1 0], got %v", best.Steps)
	}
	if best.Suppressed != 0 || len(best.Kept) != 4 {
		t.Fatalf("expected 0 suppressed, 4 kept; got %d/%d", best.Suppressed, len(best.Kept))
	}
}

func TestInsufficientSensitiveDiversity(t *testing.T) {
	// All rows share one sensitive value: l=2 can never be met, even fully
	// generalized, so search must fail.
	csv := "id,age,city,disease\nr1,20,bj,flu\nr2,25,bj,flu\nr3,60,sh,flu\nr4,65,sh,flu\n"
	ds, hs := loadTest(t, csv, testHierarchy)
	if _, err := Search(ds, hs, testCfg()); err == nil {
		t.Fatal("expected failure when sensitive diversity is unreachable")
	}
}

func TestCrossHierarchyRejected(t *testing.T) {
	// "shared" has two parents: not a tree.
	hier := `{"columns": {"age": {"value": "any", "children": [
	  {"value": "a", "children": [{"value": "shared"}]},
	  {"value": "b", "children": [{"value": "shared"}]}
	]}}}`
	if _, err := LoadHierarchies(writeTemp(t, "h.json", hier)); err == nil {
		t.Fatal("expected non-tree hierarchy to be rejected")
	}
}

func TestBudgetInfeasible(t *testing.T) {
	// k=3 forces suppression of both size-2 groups at steps 0; full
	// generalization merges into one group of 4 which passes, so instead
	// use budget 0 with k=3 and l=3 (unreachable) to force failure.
	ds, hs := loadTest(t, testCSV, testHierarchy)
	cfg := &Config{MinGroupSize: 3, MinDistinctSens: 3, Budget: 0, Weights: []int{1, 1}, SuppressCost: 1}
	if _, err := Search(ds, hs, cfg); err == nil {
		t.Fatal("expected failure when constraints are unreachable within budget")
	}
	// Budget too small: k=5 can never be met by 4 rows; every scheme
	// suppresses all 4 rows, exceeding budget 3.
	cfg = &Config{MinGroupSize: 5, MinDistinctSens: 1, Budget: 3, Weights: []int{1, 1}, SuppressCost: 1}
	if _, err := Search(ds, hs, cfg); err == nil {
		t.Fatal("expected failure when suppression exceeds budget")
	}
}

func TestWholeGroupSuppressed(t *testing.T) {
	// One group of 2 with distinct sensitive values, one group of 1:
	// the singleton group must be dropped entirely.
	csv := "id,age,city,disease\nr1,20,bj,flu\nr2,25,bj,cold\nr3,60,sh,flu\n"
	ds, hs := loadTest(t, csv, testHierarchy)
	cfg := &Config{MinGroupSize: 2, MinDistinctSens: 2, Budget: 1, Weights: []int{1, 1}, SuppressCost: 1}
	best, err := Search(ds, hs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if best.Suppressed != 1 || len(best.Kept) != 2 {
		t.Fatalf("expected 1 suppressed, 2 kept; got %d/%d", best.Suppressed, len(best.Kept))
	}
	for _, k := range best.Kept {
		if k.Sensitive == "flu" && k.QI[0] == "60" {
			t.Fatal("singleton group row should have been suppressed as a whole")
		}
	}
}

func TestInputOrderIrrelevant(t *testing.T) {
	shuffled := "id,age,city,disease\nr4,65,sh,cold\nr2,25,bj,cold\nr3,60,sh,flu\nr1,20,bj,flu\n"
	ds1, hs1 := loadTest(t, testCSV, testHierarchy)
	ds2, hs2 := loadTest(t, shuffled, testHierarchy)
	b1, err := Search(ds1, hs1, testCfg())
	if err != nil {
		t.Fatal(err)
	}
	b2, err := Search(ds2, hs2, testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b1.Steps, b2.Steps) || b1.Cost != b2.Cost || !reflect.DeepEqual(b1.Kept, b2.Kept) {
		t.Fatal("scheme selection must not depend on input row order")
	}
}

func TestTieBreakPrefersFewerSteps(t *testing.T) {
	// With suppression cost 0 weight differences aside, equal-cost schemes
	// must resolve to fewer suppressions then lexicographic steps.
	ds, hs := loadTest(t, testCSV, testHierarchy)
	cfg := &Config{MinGroupSize: 2, MinDistinctSens: 2, Budget: 4, Weights: []int{1, 1}, SuppressCost: 1}
	best, err := Search(ds, hs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(best.Steps, []int{1, 0}) {
		t.Fatalf("expected lexicographically smallest steps [1 0], got %v", best.Steps)
	}
}

func TestRejections(t *testing.T) {
	// Duplicate id.
	if _, err := LoadDataset(writeTemp(t, "d.csv", "id,age,disease\nr1,20,flu\nr1,25,cold\n")); err == nil {
		t.Fatal("expected duplicate id rejection")
	}
	// Missing value.
	if _, err := LoadDataset(writeTemp(t, "d.csv", "id,age,disease\nr1,,flu\n")); err == nil {
		t.Fatal("expected missing value rejection")
	}
	// Unknown leaf value.
	ds, err := LoadDataset(writeTemp(t, "d.csv", "id,age,city,disease\nr1,99,bj,flu\n"))
	if err != nil {
		t.Fatal(err)
	}
	hs, err := LoadHierarchies(writeTemp(t, "h.json", testHierarchy))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLeaves(ds, hs); err == nil {
		t.Fatal("expected unknown leaf rejection during validation")
	}
	if _, err := hs["age"].Generalize("99", 0); err == nil {
		t.Fatal("expected unknown leaf rejection")
	}
	// Depth over 4.
	deep := `{"columns": {"a": {"value": "r", "children": [{"value": "1", "children": [
	  {"value": "2", "children": [{"value": "3", "children": [
	    {"value": "4", "children": [{"value": "5"}]}]}]}]}]}}}`
	if _, err := LoadHierarchies(writeTemp(t, "h.json", deep)); err == nil {
		t.Fatal("expected depth>4 rejection")
	}
}
