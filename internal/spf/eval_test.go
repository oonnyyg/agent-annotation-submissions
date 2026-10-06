package spf

import (
	"strings"
	"testing"
)

func snap(records map[string]map[string][][]string, errs map[string]string) *Snapshot {
	norm := map[string]map[string][][]string{}
	for d, v := range records {
		norm[strings.ToLower(d)] = v
	}
	if errs == nil {
		errs = map[string]string{}
	}
	return &Snapshot{Records: norm, Errors: errs}
}

func txt(domain string, segments ...string) map[string]map[string][][]string {
	return map[string]map[string][][]string{
		strings.ToLower(domain): {"TXT": {segments}},
	}
}

func check(t *testing.T, s *Snapshot, ip, domain string) Result {
	t.Helper()
	return NewEvaluator(s).Check(ip, domain)
}

func TestIP4PassWithCIDR(t *testing.T) {
	s := snap(txt("Example.COM", "v=spf1", " ip4:192.0.2.0/24 -all"), nil)
	r := check(t, s, "192.0.2.7", "example.com")
	if r.Result != ResultPass || r.MatchedRule != "ip4:192.0.2.0/24" {
		t.Fatalf("got %+v", r)
	}
}

func TestQualifiers(t *testing.T) {
	cases := []struct{ rec, want string }{
		{"v=spf1 +ip4:10.0.0.1 -all", ResultPass},
		{"v=spf1 -ip4:10.0.0.1", ResultFail},
		{"v=spf1 ~ip4:10.0.0.1", ResultSoftFail},
		{"v=spf1 ?ip4:10.0.0.1", ResultNeutral},
	}
	for _, c := range cases {
		s := snap(txt("d.test", c.rec), nil)
		if r := check(t, s, "10.0.0.1", "d.test"); r.Result != c.want {
			t.Errorf("%s: got %s want %s", c.rec, r.Result, c.want)
		}
	}
}

func TestIP6AndFamilyMismatch(t *testing.T) {
	s := snap(txt("d.test", "v=spf1 ip6:2001:db8::/32 -all"), nil)
	if r := check(t, s, "2001:db8::1", "d.test"); r.Result != ResultPass {
		t.Fatalf("v6: %+v", r)
	}
	if r := check(t, s, "192.0.2.1", "d.test"); r.Result != ResultFail {
		t.Fatalf("v4 sender must not match ip6: %+v", r)
	}
}

func TestAMechanismSelectsFamily(t *testing.T) {
	recs := txt("d.test", "v=spf1 a -all")
	recs["d.test"]["A"] = [][]string{{"192.0.2.5"}}
	recs["d.test"]["AAAA"] = [][]string{{"2001:db8::5"}}
	s := snap(recs, nil)
	if r := check(t, s, "192.0.2.5", "d.test"); r.Result != ResultPass {
		t.Fatalf("A: %+v", r)
	}
	if r := check(t, s, "2001:db8::5", "d.test"); r.Result != ResultPass {
		t.Fatalf("AAAA: %+v", r)
	}
	if r := check(t, s, "192.0.2.6", "d.test"); r.Result != ResultFail {
		t.Fatalf("no match: %+v", r)
	}
}

func TestAMechanismDualCIDR(t *testing.T) {
	recs := txt("d.test", "v=spf1 a/24//64 -all")
	recs["d.test"]["A"] = [][]string{{"192.0.2.9"}}
	recs["d.test"]["AAAA"] = [][]string{{"2001:db8:1::9"}}
	s := snap(recs, nil)
	if r := check(t, s, "192.0.2.200", "d.test"); r.Result != ResultPass {
		t.Fatalf("v4 /24: %+v", r)
	}
	if r := check(t, s, "2001:db8:1::ffff", "d.test"); r.Result != ResultPass {
		t.Fatalf("v6 /64: %+v", r)
	}
}

func TestNoRecordVsMultipleRecords(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"none.test":  {"TXT": {{"some", " other"}}},
		"multi.test": {"TXT": {{"v=spf1 a"}, {"v=spf1 -all"}}},
	}, nil)
	if r := check(t, s, "10.0.0.1", "none.test"); r.Result != ResultNone {
		t.Fatalf("none: %+v", r)
	}
	if r := check(t, s, "10.0.0.1", "multi.test"); r.Result != ResultPermError {
		t.Fatalf("multi: %+v", r)
	}
}

func TestTxtSegmentsJoinedBeforeSelection(t *testing.T) {
	s := snap(map[string]map[string][][]string{
		"d.test": {"TXT": {{"v=", "spf1 ip4:10.0.0.1"}}},
	}, nil)
	if r := check(t, s, "10.0.0.1", "d.test"); r.Result != ResultPass {
		t.Fatalf("%+v", r)
	}
}

func TestNeutralWithoutRedirect(t *testing.T) {
	s := snap(txt("d.test", "v=spf1 ip4:10.0.0.1"), nil)
	r := check(t, s, "10.0.0.2", "d.test")
	if r.Result != ResultNeutral || r.MatchedRule != "" {
		t.Fatalf("%+v", r)
	}
}

func TestUnsupportedMechanismIsPermError(t *testing.T) {
	for _, rec := range []string{
		"v=spf1 mx -all",
		"v=spf1 ptr -all",
		"v=spf1 exists:%{i}.x.test",
		"v=spf1 ip4:%{i} -all",
		"v=spf1 exp=msg.test -all",
	} {
		s := snap(txt("d.test", rec), nil)
		r := check(t, s, "10.0.0.1", "d.test")
		if r.Result != ResultPermError {
			t.Errorf("%s: got %s, want permerror", rec, r.Result)
		}
	}
}
