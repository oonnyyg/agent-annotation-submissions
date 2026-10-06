package spf

import "testing"

func TestIncludePassMatches(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"d.test":     {"TXT": {{"v=spf1 include:child.test -all"}}},
		"child.test": {"TXT": {{"v=spf1 ip4:10.0.0.1 -all"}}},
	}, nil)
	r := check(t, s, "10.0.0.1", "d.test")
	if r.Result != ResultPass || r.MatchedRule != "include:child.test" {
		t.Fatalf("%+v", r)
	}
}

func TestIncludeChildFailDoesNotRejectParent(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"d.test":     {"TXT": {{"v=spf1 include:child.test ip4:10.9.9.9 -all"}}},
		"child.test": {"TXT": {{"v=spf1 -ip4:10.0.0.1"}}},
	}, nil)
	// Child says fail for 10.0.0.1; parent must keep evaluating and
	// reach its own -all, not inherit a pass. Sender 10.9.9.9 passes
	// via the parent's own mechanism.
	if r := check(t, s, "10.0.0.1", "d.test"); r.Result != ResultFail {
		t.Fatalf("child fail leaked as parent pass: %+v", r)
	}
	if r := check(t, s, "10.9.9.9", "d.test"); r.Result != ResultPass {
		t.Fatalf("parent mechanism skipped: %+v", r)
	}
}

func TestIncludeMissingRecordIsPermError(t *testing.T) {
	s := snap(txt("d.test", "v=spf1 include:ghost.test -all"), nil)
	if r := check(t, s, "10.0.0.1", "d.test"); r.Result != ResultPermError {
		t.Fatalf("%+v", r)
	}
}

func TestRedirectOnlyWhenNoAll(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"withall.test": {"TXT": {{"v=spf1 ip4:10.0.0.1 -all redirect=other.test"}}},
		"noall.test":   {"TXT": {{"v=spf1 ip4:10.0.0.1 redirect=other.test"}}},
		"other.test":   {"TXT": {{"v=spf1 ip4:10.0.0.2 -all"}}},
	}, nil)
	// -all present: redirect must be ignored.
	if r := check(t, s, "10.0.0.2", "withall.test"); r.Result != ResultFail {
		t.Fatalf("redirect wrongly applied with all: %+v", r)
	}
	// No all: redirect applies after mechanisms miss.
	if r := check(t, s, "10.0.0.2", "noall.test"); r.Result != ResultPass {
		t.Fatalf("redirect not applied: %+v", r)
	}
}

func TestRedirectToMissingRecordIsPermError(t *testing.T) {
	s := snap(txt("d.test", "v=spf1 ip4:10.0.0.1 redirect=ghost.test"), nil)
	if r := check(t, s, "10.0.0.2", "d.test"); r.Result != ResultPermError {
		t.Fatalf("%+v", r)
	}
}

func TestRedirectChainPropagatesResult(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"a.test": {"TXT": {{"v=spf1 redirect=b.test"}}},
		"b.test": {"TXT": {{"v=spf1 redirect=c.test"}}},
		"c.test": {"TXT": {{"v=spf1 ~all"}}},
	}, nil)
	if r := check(t, s, "10.0.0.1", "a.test"); r.Result != ResultSoftFail {
		t.Fatalf("%+v", r)
	}
}

func TestIncludeLoopTerminates(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"a.test": {"TXT": {{"v=spf1 include:b.test -all"}}},
		"b.test": {"TXT": {{"v=spf1 include:a.test -all"}}},
	}, nil)
	r := check(t, s, "10.0.0.1", "a.test")
	if r.Result != ResultPermError {
		t.Fatalf("loop should be permerror: %+v", r)
	}
}

func TestTempErrorVsPermError(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"flaky.test": {"TXT": {{"v=spf1 -all"}}},
		"via.test":   {"TXT": {{"v=spf1 include:flaky.test -all"}}},
	}, map[string]string{"flaky.test": "SERVFAIL"})
	if r := check(t, s, "10.0.0.1", "flaky.test"); r.Result != ResultTempError {
		t.Fatalf("direct: %+v", r)
	}
	r := check(t, s, "10.0.0.1", "via.test")
	if r.Result != ResultTempError {
		t.Fatalf("temperror must propagate through include: %+v", r)
	}
}

func TestPermErrorPropagatesThroughInclude(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"bad.test": {"TXT": {{"v=spf1 mx"}}},
		"via.test": {"TXT": {{"v=spf1 include:bad.test -all"}}},
	}, nil)
	if r := check(t, s, "10.0.0.1", "via.test"); r.Result != ResultPermError {
		t.Fatalf("%+v", r)
	}
}
