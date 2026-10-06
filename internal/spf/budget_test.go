package spf

import (
	"fmt"
	"testing"
)

func includeChain(n int) *Snapshot {
	recs := map[string]map[string][][]string{}
	for i := 0; i < n; i++ {
		next := fmt.Sprintf("d%d.test", i+1)
		recs[fmt.Sprintf("d%d.test", i)] = map[string][][]string{
			"TXT": {{"v=spf1 include:" + next + " -all"}},
		}
	}
	recs[fmt.Sprintf("d%d.test", n)] = map[string][][]string{
		"TXT": {{"v=spf1 ip4:10.0.0.1 -all"}},
	}
	return snap(recs, nil)
}

func TestLookupBudgetBoundary(t *testing.T) {
	// 9 includes + final record = 10 TXT lookups: exactly at budget.
	if r := check(t, includeChain(9), "10.0.0.1", "d0.test"); r.Result != ResultPass {
		t.Fatalf("at budget should pass: %+v", r)
	}
	// 10 includes + final record = 11 lookups: over budget.
	r := check(t, includeChain(10), "10.0.0.1", "d0.test")
	if r.Result != ResultPermError {
		t.Fatalf("over budget should be permerror: %+v", r)
	}
	if r.BudgetUsed != MaxLookups+1 {
		t.Fatalf("budget used = %d, want %d", r.BudgetUsed, MaxLookups+1)
	}
}

func TestCacheHitsStillConsumeBudget(t *testing.T) {
	// Same child included repeatedly: cached answers must still
	// consume budget. Parent TXT + 10 child lookups = 11 > 10.
	recs := map[string]map[string][][]string{
		"child.test": {"TXT": {{"v=spf1 ip4:10.0.0.1"}}},
	}
	parent := "v=spf1"
	for i := 0; i < 10; i++ {
		parent += " include:child.test"
	}
	parent += " -all"
	recs["d.test"] = map[string][][]string{"TXT": {{parent}}}
	r := check(t, snap(recs, nil), "10.0.0.2", "d.test")
	if r.Result != ResultPermError {
		t.Fatalf("cached lookups must count toward budget: %+v", r)
	}
}

func TestVoidLookupLimit(t *testing.T) {
	recs := map[string]map[string][][]string{
		"d.test": {"TXT": {{"v=spf1 a:x.test a:y.test a:z.test -all"}}},
	}
	r := check(t, snap(recs, nil), "10.0.0.1", "d.test")
	if r.Result != ResultPermError {
		t.Fatalf("third void lookup should be permerror: %+v", r)
	}
	if r.VoidLookups != MaxVoidLookups+1 {
		t.Fatalf("voids = %d, want %d", r.VoidLookups, MaxVoidLookups+1)
	}
}

func TestVoidBoundaryOk(t *testing.T) {
	recs := map[string]map[string][][]string{
		"d.test": {"TXT": {{"v=spf1 a:x.test a:y.test ip4:10.0.0.1 -all"}}},
	}
	if r := check(t, snap(recs, nil), "10.0.0.1", "d.test"); r.Result != ResultPass {
		t.Fatalf("two voids are allowed: %+v", r)
	}
}

func TestBudgetIsPerTopLevelQuery(t *testing.T) {
	s := includeChain(9)
	ev := NewEvaluator(s)
	for i := 0; i < 3; i++ {
		if r := ev.Check("10.0.0.1", "d0.test"); r.Result != ResultPass {
			t.Fatalf("query %d: budget leaked across queries: %+v", i, r)
		}
	}
}
