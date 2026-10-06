package spf

import (
	"strings"
	"testing"
)

func mustSnap(t *testing.T, js string) *Snapshot {
	t.Helper()
	s, err := ParseSnapshot([]byte(js))
	if err != nil {
		t.Fatalf("ParseSnapshot: %v", err)
	}
	return s
}

func check(t *testing.T, s *Snapshot, ip, domain string) Evaluation {
	t.Helper()
	return NewEvaluator(s).Check(ip, domain)
}

func TestIP4PassAndQualifiers(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"pass.example": {"TXT":[["v=spf1"," ip4:192.0.2.0/24 -all"]]},
		"soft.example": {"TXT":[["v=spf1 ~ip4:192.0.2.5"]]},
		"neut.example": {"TXT":[["v=spf1 ?ip4:192.0.2.5"]]},
		"fail.example": {"TXT":[["v=spf1 -ip4:192.0.2.5"]]}
	}}`)
	cases := []struct {
		domain string
		want   Result
	}{
		{"pass.example", ResultPass},
		{"soft.example", ResultSoftFail},
		{"neut.example", ResultNeutral},
		{"fail.example", ResultFail},
	}
	for _, c := range cases {
		ev := check(t, s, "192.0.2.5", c.domain)
		if ev.Result != c.want {
			t.Errorf("%s: got %s want %s", c.domain, ev.Result, c.want)
		}
	}
	// -all after non-matching ip4 -> fail
	ev := check(t, s, "198.51.100.1", "pass.example")
	if ev.Result != ResultFail || ev.Matched != "-all" {
		t.Errorf("pass.example other ip: got %s matched=%q", ev.Result, ev.Matched)
	}
}

func TestIP6AndDualCIDR(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"v6.example": {"TXT":[["v=spf1 ip6:2001:db8::/32 -all"]]},
		"dual.example": {"TXT":[["v=spf1 a/24//64 -all"]],
			"A":["192.0.2.1"], "AAAA":["2001:db8:1::1"]}
	}}`)
	if ev := check(t, s, "2001:db8::5", "v6.example"); ev.Result != ResultPass {
		t.Errorf("v6 pass: got %s", ev.Result)
	}
	if ev := check(t, s, "2001:db9::5", "v6.example"); ev.Result != ResultFail {
		t.Errorf("v6 fail: got %s", ev.Result)
	}
	// a with dual cidr, v4 sender
	if ev := check(t, s, "192.0.2.99", "dual.example"); ev.Result != ResultPass {
		t.Errorf("dual v4: got %s", ev.Result)
	}
	// a with dual cidr, v6 sender uses AAAA and //64
	if ev := check(t, s, "2001:db8:1::abcd", "dual.example"); ev.Result != ResultPass {
		t.Errorf("dual v6: got %s", ev.Result)
	}
	if ev := check(t, s, "2001:db8:2::1", "dual.example"); ev.Result != ResultFail {
		t.Errorf("dual v6 miss: got %s", ev.Result)
	}
}

func TestAMechanismFamilySelection(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"a.example": {"TXT":[["v=spf1 a -all"]],
			"A":["203.0.113.9"], "AAAA":["2001:db8::9"]}
	}}`)
	if ev := check(t, s, "203.0.113.9", "a.example"); ev.Result != ResultPass {
		t.Errorf("v4 a: got %s", ev.Result)
	}
	if ev := check(t, s, "2001:db8::9", "a.example"); ev.Result != ResultPass {
		t.Errorf("v6 a: got %s", ev.Result)
	}
	if ev := check(t, s, "203.0.113.10", "a.example"); ev.Result != ResultFail {
		t.Errorf("v4 a miss: got %s", ev.Result)
	}
}

func TestTXTConcatAndCaseInsensitive(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"MiXeD.Example": {"TXT":[["v=sp","f1 ip4:192.0.2.1"],["not spf"]]}
	}}`)
	ev := check(t, s, "192.0.2.1", "mixed.example")
	if ev.Result != ResultPass {
		t.Fatalf("concat/case: got %s (%s)", ev.Result, ev.Reason)
	}
	if !strings.Contains(ev.Evidence[0].Detail, "2 TXT") {
		t.Errorf("expected 2 TXT records counted, got %+v", ev.Evidence)
	}
}

func TestNoRecordVsMultiple(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"none.example": {"TXT":[["something else"]]},
		"multi.example": {"TXT":[["v=spf1 a"],["v=spf1 -all"]]}
	}}`)
	if ev := check(t, s, "192.0.2.1", "none.example"); ev.Result != ResultNone {
		t.Errorf("none: got %s", ev.Result)
	}
	if ev := check(t, s, "192.0.2.1", "multi.example"); ev.Result != ResultPermError {
		t.Errorf("multi: got %s", ev.Result)
	}
	if ev := check(t, s, "192.0.2.1", "absent.example"); ev.Result != ResultNone {
		t.Errorf("absent: got %s", ev.Result)
	}
}

func TestUnsupportedMechanismAndMacro(t *testing.T) {
	s := mustSnap(t, `{"zones":{
		"mx.example": {"TXT":[["v=spf1 mx -all"]]},
		"macro.example": {"TXT":[["v=spf1 exists:%{i}.x.example -all"]]}
	}}`)
	for _, d := range []string{"mx.example", "macro.example"} {
		ev := check(t, s, "192.0.2.1", d)
		if ev.Result != ResultPermError {
			t.Errorf("%s: got %s want permerror", d, ev.Result)
		}
		if !strings.Contains(ev.Reason, "out of scope") {
			t.Errorf("%s: reason %q should mention out of scope", d, ev.Reason)
		}
	}
}
