package natreplay

import "testing"

// 超时边界：expiresAt <= now 即清理；超时后端口确定性复用，旧允许列表丢弃。
func TestTimeoutBoundaryAndReuse(t *testing.T) {
	cfg := testConfig()
	cfg.Pool = []PoolEntry{{IP: "203.0.113.10", PortStart: 10000, PortEnd: 10000}}
	cfg.ReservedPorts = nil
	results, _ := runEvents(t, cfg, []Event{
		out(0, "10.0.0.1", 5000, "8.8.8.8", 53), // 到期时刻 1000
		in(999, "8.8.8.8", 53, "203.0.113.10", 10000),
		in(1000, "8.8.8.8", 53, "203.0.113.10", 10000), // 恰到期，先清理
		out(1000, "10.0.0.2", 6000, "1.1.1.1", 443),    // 复用同一端口
		in(1001, "8.8.8.8", 53, "203.0.113.10", 10000), // 旧允许列表已丢弃
		in(1001, "1.1.1.1", 443, "203.0.113.10", 10000),
	})
	if results[1].Action != "forward" {
		t.Fatalf("t=999 must be alive: %+v", results[1])
	}
	if results[2].Reason != "unmapped" {
		t.Fatalf("t=1000 must expire: %+v", results[2])
	}
	if results[3].Public != "203.0.113.10:10000" {
		t.Fatalf("reuse = %+v", results[3])
	}
	if results[4].Reason != "filtered" {
		t.Fatalf("stale allowlist must be dropped: %+v", results[4])
	}
	if results[5].Action != "forward" {
		t.Fatalf("new allowlist entry: %+v", results[5])
	}
}

// 入站不延长空闲期限；查询同样推进清理。
func TestInboundDoesNotRefreshAndQueryExpires(t *testing.T) {
	cfg := testConfig()
	cfg.Pool = []PoolEntry{{IP: "203.0.113.10", PortStart: 10000, PortEnd: 10000}}
	cfg.ReservedPorts = nil
	results, sum := runEvents(t, cfg, []Event{
		out(0, "10.0.0.1", 5000, "8.8.8.8", 53),
		in(500, "8.8.8.8", 53, "203.0.113.10", 10000),
		in(900, "8.8.8.8", 53, "203.0.113.10", 10000),
		in(1000, "8.8.8.8", 53, "203.0.113.10", 10000), // 到期，入站未续期
		{Type: "query", Time: 2000},
	})
	if results[3].Reason != "unmapped" {
		t.Fatalf("inbound must not refresh: %+v", results[3])
	}
	if results[4].Action != "query" {
		t.Fatalf("query = %+v", results[4])
	}
	if len(sum.Active) != 0 {
		t.Fatalf("query should have expired everything: %+v", sum.Active)
	}
}

// open 模式：任何来源都可回包；入站不能创建映射。
func TestOpenModeAndInboundCannotCreate(t *testing.T) {
	cfg := testConfig()
	cfg.InboundMode = "open"
	results, _ := runEvents(t, cfg, []Event{
		in(0, "8.8.8.8", 53, "203.0.113.10", 10000), // 无映射
		out(0, "10.0.0.1", 5000, "8.8.8.8", 53),
		in(1, "9.9.9.9", 1234, "203.0.113.10", 10000), // open 模式放行
	})
	if results[0].Reason != "unmapped" {
		t.Fatalf("inbound must not create mapping: %+v", results[0])
	}
	if results[2].Action != "forward" {
		t.Fatalf("open mode must forward: %+v", results[2])
	}
}

// 重复回放结果一致。
func TestDeterministicReplay(t *testing.T) {
	evs := []Event{
		out(0, "10.0.0.1", 5000, "8.8.8.8", 53),
		out(0, "10.0.0.2", 5000, "1.1.1.1", 443),
		in(10, "8.8.8.8", 53, "203.0.113.10", 10000),
		in(2000, "8.8.8.8", 53, "203.0.113.10", 10000),
	}
	r1, s1 := runEvents(t, testConfig(), evs)
	r2, s2 := runEvents(t, testConfig(), evs)
	if mustJSON(r1) != mustJSON(r2) || mustJSON(s1) != mustJSON(s2) {
		t.Fatal("replay not deterministic")
	}
}
