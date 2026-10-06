package nat

import "testing"

func testConfig() Config {
	return Config{
		PublicAddresses: []string{"203.0.113.10"},
		ReservedPorts:   []string{"0-1023"},
		IdleTimeoutMs:   1000,
		Mode:            "restricted",
	}
}

func mustTable(t *testing.T, cfg Config) *Table {
	t.Helper()
	tb, err := NewTable(cfg)
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	return tb
}

func outbound(tb *Table, time int64, srcIP string, srcPort int, dstIP string, dstPort int) Result {
	res, err := tb.Process(Event{Time: time, Type: "outbound", SrcIP: srcIP, SrcPort: srcPort, DstIP: dstIP, DstPort: dstPort})
	if err != nil {
		panic(err)
	}
	return res
}

func inbound(tb *Table, time int64, srcIP string, srcPort int, dstPort int) Result {
	res, err := tb.Process(Event{Time: time, Type: "inbound", SrcIP: srcIP, SrcPort: srcPort, DstIP: "203.0.113.10", DstPort: dstPort})
	if err != nil {
		panic(err)
	}
	return res
}

func TestReuseAcrossDestinations(t *testing.T) {
	tb := mustTable(t, testConfig())
	r1 := outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	if !r1.Created || r1.Mapping.PublicPort != 5000 {
		t.Fatalf("first outbound: %+v", r1)
	}
	r2 := outbound(tb, 100, "10.0.0.1", 5000, "1.1.1.1", 443)
	if r2.Created || r2.Mapping.PublicPort != 5000 {
		t.Fatalf("mapping not reused: %+v", r2)
	}
	if r2.Mapping.ExpiresAt != 1100 {
		t.Fatalf("deadline not refreshed: %+v", r2.Mapping)
	}
}

func TestSourcePortContention(t *testing.T) {
	tb := mustTable(t, testConfig())
	outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	r := outbound(tb, 0, "10.0.0.2", 5000, "8.8.8.8", 53)
	if r.Mapping.PublicPort != 1024 {
		t.Fatalf("expected fallback port 1024, got %+v", r.Mapping)
	}
}

func TestAddressTieBreak(t *testing.T) {
	cfg := testConfig()
	cfg.PublicAddresses = []string{"203.0.113.11", "203.0.113.10"}
	cfg.ReservedPorts = []string{"0-65534"}
	tb := mustTable(t, cfg)
	r := outbound(tb, 0, "10.0.0.1", 5000, "8.8.8.8", 53)
	if r.Mapping.PublicIP != "203.0.113.10" || r.Mapping.PublicPort != 65535 {
		t.Fatalf("expected lowest address with only free port, got %+v", r.Mapping)
	}
}
