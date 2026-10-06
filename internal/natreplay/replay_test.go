package natreplay

import "testing"

func testConfig() Config {
	return Config{
		Pool: []PoolEntry{
			{IP: "203.0.113.10", PortStart: 10000, PortEnd: 10002},
			{IP: "203.0.113.11", PortStart: 10000, PortEnd: 10000},
		},
		ReservedPorts: []uint16{10002},
		IdleTimeoutMS: 1000,
		InboundMode:   "restricted",
	}
}

func runEvents(t *testing.T, cfg Config, evs []Event) ([]Result, Summary) {
	t.Helper()
	table, err := NewTable(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rep := NewReplayer(table)
	var results []Result
	for _, ev := range evs {
		res, err := rep.Handle(ev)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, res)
	}
	return results, rep.Summary()
}

func out(t int64, sip string, sport uint16, dip string, dport uint16) Event {
	return Event{Type: "outbound", Time: t, SrcIP: sip, SrcPort: sport, DstIP: dip, DstPort: dport}
}

func in(t int64, sip string, sport uint16, dip string, dport uint16) Event {
	return Event{Type: "inbound", Time: t, SrcIP: sip, SrcPort: sport, DstIP: dip, DstPort: dport}
}

// 同一内网端点到不同外部目标复用同一映射。
func TestMappingReuseAcrossDestinations(t *testing.T) {
	results, sum := runEvents(t, testConfig(), []Event{
		out(0, "10.0.0.1", 5000, "8.8.8.8", 53),
		out(10, "10.0.0.1", 5000, "1.1.1.1", 443),
	})
	if results[0].Public != "203.0.113.10:10000" {
		t.Fatalf("first mapping = %s", results[0].Public)
	}
	if results[1].Public != results[0].Public {
		t.Fatalf("expected reuse, got %s then %s", results[0].Public, results[1].Public)
	}
	if sum.ForwardedOut != 2 || len(sum.Active) != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(sum.Active[0].Allowed) != 2 {
		t.Fatalf("allowed = %v", sum.Active[0].Allowed)
	}
}

// 源端口被占用时按 (地址数值, 端口) 升序选空位；保留端口不可分配。
func TestAllocationOrderAndReserved(t *testing.T) {
	results, _ := runEvents(t, testConfig(), []Event{
		out(0, "10.0.0.1", 10000, "8.8.8.8", 53), // 占用 203.0.113.10:10000
		out(0, "10.0.0.2", 10000, "8.8.8.8", 53), // 保留源端口，选 203.0.113.11:10000
		out(0, "10.0.0.3", 10000, "8.8.8.8", 53), // 10002 保留，选 203.0.113.10:10001
		out(0, "10.0.0.4", 10000, "8.8.8.8", 53), // 池耗尽
	})
	want := []string{"203.0.113.10:10000", "203.0.113.11:10000", "203.0.113.10:10001"}
	for i, w := range want {
		if results[i].Public != w {
			t.Fatalf("event %d public = %s, want %s", i, results[i].Public, w)
		}
	}
	if results[3].Action != "drop" || results[3].Reason != "no_capacity" {
		t.Fatalf("event 3 = %+v", results[3])
	}
}

// 容量耗尽拒绝新出站但保留现有映射；失败不添加目标、不刷新期限。
func TestCapacityExhaustionKeepsExisting(t *testing.T) {
	cfg := testConfig()
	cfg.Pool = []PoolEntry{{IP: "203.0.113.10", PortStart: 10000, PortEnd: 10000}}
	cfg.ReservedPorts = nil
	results, sum := runEvents(t, cfg, []Event{
		out(0, "10.0.0.1", 5000, "8.8.8.8", 53),
		out(500, "10.0.0.2", 5000, "9.9.9.9", 53), // 拒绝
		in(900, "8.8.8.8", 53, "203.0.113.10", 10000),
		in(900, "9.9.9.9", 53, "203.0.113.10", 10000), // 未授权，被过滤
	})
	if results[1].Reason != "no_capacity" {
		t.Fatalf("event 1 = %+v", results[1])
	}
	if results[2].Action != "forward" {
		t.Fatalf("existing mapping must survive: %+v", results[2])
	}
	if results[3].Reason != "filtered" {
		t.Fatalf("failed outbound must not authorize: %+v", results[3])
	}
	if sum.ForwardedOut != 1 || sum.ForwardedIn != 1 {
		t.Fatalf("summary = %+v", sum)
	}
}
