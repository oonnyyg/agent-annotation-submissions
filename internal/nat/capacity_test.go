package nat

import "testing"

func TestCapacityExhausted(t *testing.T) {
	cfg := testConfig()
	cfg.ReservedPorts = []string{"0-4999", "5001-65535"}
	tb := mustTable(t, cfg)
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	r := outbound(tb, 10, "10.0.0.2", 5000, "8.8.8.8", 53)
	if r.Action != "dropped" || r.Reason != "capacity_exhausted" {
		t.Fatalf("expected capacity_exhausted: %+v", r)
	}
	if r := inbound(tb, 20, "8.8.8.8", 53, 5000); r.Action != "forwarded" {
		t.Fatalf("existing mapping must survive exhaustion: %+v", r)
	}
	if tb.Summary().Drops["capacity_exhausted"] != 1 {
		t.Fatalf("summary drops: %+v", tb.Summary())
	}
}

func TestFailedOutboundNoSideEffects(t *testing.T) {
	cfg := testConfig()
	cfg.ReservedPorts = []string{"0-4999", "5001-65535"}
	tb := mustTable(t, cfg)
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	outbound(tb, 10, "10.0.0.2", 5000, "1.1.1.1", 443)
	if r := inbound(tb, 20, "1.1.1.1", 443, 5000); r.Reason != "filtered" {
		t.Fatalf("failed outbound must not add allowlist target: %+v", r)
	}
	if r := inbound(tb, 1010, "8.8.8.8", 53, 5000); r.Reason != "no_mapping" {
		t.Fatalf("failed outbound must not refresh deadline: %+v", r)
	}
}

func TestDeterministicReplay(t *testing.T) {
	events := []Event{
		{Time: 0, Type: "outbound", SrcIP: "10.0.0.1", SrcPort: 5000, DstIP: "8.8.8.8", DstPort: 53},
		{Time: 0, Type: "outbound", SrcIP: "10.0.0.2", SrcPort: 5000, DstIP: "8.8.8.8", DstPort: 53},
		{Time: 500, Type: "inbound", SrcIP: "8.8.8.8", SrcPort: 53, DstIP: "203.0.113.10", DstPort: 1024},
		{Time: 1000, Type: "query"},
		{Time: 1001, Type: "outbound", SrcIP: "10.0.0.3", SrcPort: 5000, DstIP: "9.9.9.9", DstPort: 53},
	}
	run := func() ([]Result, Summary) {
		tb := mustTable(t, testConfig())
		out := make([]Result, 0, len(events))
		for _, ev := range events {
			r, err := tb.Process(ev)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, r)
		}
		return out, tb.Summary()
	}
	r1, s1 := run()
	r2, s2 := run()
	if len(r1) != len(r2) {
		t.Fatal("replay length mismatch")
	}
	for i := range r1 {
		if r1[i].Action != r2[i].Action || r1[i].Reason != r2[i].Reason {
			t.Fatalf("replay diverged at %d: %+v vs %+v", i, r1[i], r2[i])
		}
	}
	if s1.OutboundForward != s2.OutboundForward || s1.InboundForward != s2.InboundForward {
		t.Fatalf("summary diverged: %+v vs %+v", s1, s2)
	}
	if s1.OutboundForward != 3 || s1.InboundForward != 1 {
		t.Fatalf("unexpected summary: %+v", s1)
	}
}
