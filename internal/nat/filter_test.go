package nat

import "testing"

func TestRestrictedFiltering(t *testing.T) {
	tb := mustTable(t, testConfig())
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	if r := inbound(tb, 10, "8.8.8.8", 53, 5000); r.Action != "forwarded" {
		t.Fatalf("contacted peer should pass: %+v", r)
	}
	if r := inbound(tb, 10, "8.8.8.8", 5353, 5000); r.Reason != "filtered" {
		t.Fatalf("wrong source port should be filtered: %+v", r)
	}
	if r := inbound(tb, 10, "9.9.9.9", 53, 5000); r.Reason != "filtered" {
		t.Fatalf("unknown peer should be filtered: %+v", r)
	}
	if r := inbound(tb, 10, "8.8.8.8", 53, 6000); r.Reason != "no_mapping" {
		t.Fatalf("unmapped port should be no_mapping: %+v", r)
	}
}

func TestInboundDoesNotRefreshOrAuthorize(t *testing.T) {
	tb := mustTable(t, testConfig())
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	inbound(tb, 900, "8.8.8.8", 53, 5000)
	if r := inbound(tb, 1001, "8.8.8.8", 53, 5000); r.Reason != "no_mapping" {
		t.Fatalf("inbound must not extend lifetime: %+v", r)
	}
	outbound(tb, 2000, "10.0.0.1", 5000, "8.8.8.8", 53)
	inbound(tb, 2001, "7.7.7.7", 80, 5000)
	if r := inbound(tb, 2002, "7.7.7.7", 80, 5000); r.Reason != "filtered" {
		t.Fatalf("inbound must not authorize itself: %+v", r)
	}
}

func TestOpenMode(t *testing.T) {
	cfg := testConfig()
	cfg.Mode = "open"
	tb := mustTable(t, cfg)
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	if r := inbound(tb, 10, "9.9.9.9", 1234, 5000); r.Action != "forwarded" {
		t.Fatalf("open mode should forward any inbound: %+v", r)
	}
}

func TestExpiryBoundaryAndReuse(t *testing.T) {
	tb := mustTable(t, testConfig())
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	if r := inbound(tb, 999, "8.8.8.8", 53, 5000); r.Action != "forwarded" {
		t.Fatalf("mapping should live until deadline: %+v", r)
	}
	if r := inbound(tb, 1000, "8.8.8.8", 53, 5000); r.Reason != "no_mapping" {
		t.Fatalf("deadline not later than now must expire: %+v", r)
	}
	r := outbound(tb, 1000, "10.0.0.2", 5000, "1.1.1.1", 443)
	if r.Mapping.PublicPort != 5000 {
		t.Fatalf("freed port should be reused deterministically: %+v", r.Mapping)
	}
	if got := inbound(tb, 1001, "8.8.8.8", 53, 5000); got.Reason != "filtered" {
		t.Fatalf("stale allowlist must be discarded: %+v", got)
	}
}

func TestQueryAdvancesCleanup(t *testing.T) {
	tb := mustTable(t, testConfig())
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	res, err := tb.Process(Event{Time: 1000, Type: "query"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Active) != 0 {
		t.Fatalf("query should observe expired mappings cleaned: %+v", res.Active)
	}
}
