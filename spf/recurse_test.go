package spf

import (
	"strings"
	"testing"
)

func TestIncludePassAndChildFail(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"parent.example": {"TXT":[["v=spf1 include:ok.example include:bad.example -all"]]},
		"ok.example":     {"TXT":[["v=spf1 ip4:192.0.2.0/24"]]},
		"bad.example":    {"TXT":[["v=spf1 -all"]]},
		"only.example":   {"TXT":[["v=spf1 include:bad.example ip4:203.0.113.1 -all"]]}
	}}`)
	// include matches only when child passes.
	if ev := check(t, s, "192.0.2.9", "parent.example"); ev.Result != ResultPass {
		t.Fatalf("include pass: got %s", ev.Result)
	}
	// Child fail must not fail the parent; next mechanism decides.
	ev := check(t, s, "203.0.113.1", "only.example")
	if ev.Result != ResultPass || ev.Matched != "ip4:203.0.113.1" {
		t.Fatalf("child fail not parent fail: got %s matched=%q", ev.Result, ev.Matched)
	}
	// Child fail + no other match -> parent -all fires.
	if ev := check(t, s, "198.51.100.7", "only.example"); ev.Result != ResultFail {
		t.Fatalf("fallthrough: got %s", ev.Result)
	}
}

func TestRedirectSemantics(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"redir.example":  {"TXT":[["v=spf1 ip4:10.0.0.1 redirect=t.example"]]},
		"t.example":      {"TXT":[["v=spf1 ip4:192.0.2.0/24 -all"]]},
		"allwins.example":{"TXT":[["v=spf1 redirect=t.example ?all"]]},
		"missing.example":{"TXT":[["v=spf1 redirect=absent.example"]]}
	}}`)
	// redirect applies only when no rule matched.
	if ev := check(t, s, "192.0.2.3", "redir.example"); ev.Result != ResultPass {
		t.Fatalf("redirect pass: got %s", ev.Result)
	}
	if ev := check(t, s, "203.0.113.8", "redir.example"); ev.Result != ResultFail {
		t.Fatalf("redirect chain fail: got %s", ev.Result)
	}
	// all present -> redirect ignored, ?all -> neutral.
	if ev := check(t, s, "192.0.2.3", "allwins.example"); ev.Result != ResultNeutral {
		t.Fatalf("all beats redirect: got %s", ev.Result)
	}
	// redirect target without SPF record -> permerror.
	if ev := check(t, s, "192.0.2.3", "missing.example"); ev.Result != ResultPermError {
		t.Fatalf("redirect missing: got %s", ev.Result)
	}
	// no match, no redirect -> neutral.
	s2 := mustSnap(t, `{"zones":{"n.example":{"TXT":[["v=spf1 ip4:10.0.0.1"]]}}}`)
	if ev := check(t, s2, "192.0.2.3", "n.example"); ev.Result != ResultNeutral {
		t.Fatalf("neutral: got %s", ev.Result)
	}
}

func TestLoopTerminates(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"a.example": {"TXT":[["v=spf1 include:b.example -all"]]},
		"b.example": {"TXT":[["v=spf1 include:a.example -all"]]},
		"self.example": {"TXT":[["v=spf1 redirect=self.example"]]}
	}}`)
	if ev := check(t, s, "192.0.2.1", "a.example"); ev.Result != ResultPermError {
		t.Fatalf("include loop: got %s", ev.Result)
	}
	if ev := check(t, s, "192.0.2.1", "self.example"); ev.Result != ResultPermError {
		t.Fatalf("redirect loop: got %s", ev.Result)
	}
}

func TestTempVsPermErrorIsolation(t *testing.T) {
	s := mustSnap(t, `{
		"zones":{
			"flaky.example": {"TXT":[["v=spf1 -all"]]},
			"ok.example":    {"TXT":[["v=spf1 ip4:192.0.2.1 -all"]]}
		},
		"errors":{"TXT:flaky.example":"temperror"}
	}`)
	ev := NewEvaluator(s)
	if got := ev.Check("192.0.2.1", "flaky.example"); got.Result != ResultTempError {
		t.Fatalf("temperror: got %s", got.Result)
	}
	// A temperror on one query must not affect the next batch item.
	if got := ev.Check("192.0.2.1", "ok.example"); got.Result != ResultPass {
		t.Fatalf("batch isolation: got %s", got.Result)
	}
}

func TestBudgetExhaustion(t *testing.T) {
	// Chain of includes: each level costs 1 TXT lookup; 11 levels exceed 10.
	var b strings.Builder
	b.WriteString(`{"zones":{`)
	for i := 0; i <= 11; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		next := `"v=spf1 -all"`
		if i < 11 {
			next = `"v=spf1 include:d` + itoa(i+1) + `.example -all"`
		}
		b.WriteString(`"d` + itoa(i) + `.example":{"TXT":[[` + next + `]]}`)
	}
	b.WriteString(`}}`)
	s := mustSnap(t, b.String())
	ev := check(t, s, "192.0.2.1", "d0.example")
	if ev.Result != ResultPermError || !strings.Contains(ev.Reason, "lookup limit") {
		t.Fatalf("budget: got %s reason=%q", ev.Result, ev.Reason)
	}
	if ev.Lookups != MaxLookups {
		t.Fatalf("lookups used = %d, want %d", ev.Lookups, MaxLookups)
	}
	// A 10-level chain fits exactly in the budget.
	s2 := mustSnap(t, strings.Replace(b.String(),
		`"v=spf1 include:d10.example -all"`, `"v=spf1 -all"`, 1))
	s2.Zones["d11"] = Zone{}
	ev2 := check(t, s2, "192.0.2.1", "d0.example")
	if ev2.Result != ResultFail || ev2.Lookups != MaxLookups {
		t.Fatalf("budget edge: got %s lookups=%d", ev2.Result, ev2.Lookups)
	}
}

func TestVoidLimit(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"top.example": {"TXT":[["v=spf1 a:x1.example a:x2.example a:x3.example -all"]]}
	}}`)
	// Three empty address lookups exceed the void allowance of 2.
	ev := check(t, s, "192.0.2.1", "top.example")
	if ev.Result != ResultPermError || !strings.Contains(ev.Reason, "void") {
		t.Fatalf("void: got %s reason=%q", ev.Result, ev.Reason)
	}
}

func TestCacheHitStillChargesBudget(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"shared.example": {"TXT":[["v=spf1 ip4:10.0.0.1"]]},
		"top.example": {"TXT":[["v=spf1 include:shared.example include:shared.example -all"]]}
	}}`)
	ev := check(t, s, "192.0.2.1", "top.example")
	// top TXT + two include lookups = 3, even though the 2nd hits cache.
	if ev.Lookups != 3 {
		t.Fatalf("cache hit bypassed budget: lookups=%d", ev.Lookups)
	}
	if ev.Result != ResultFail {
		t.Fatalf("result: got %s", ev.Result)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
