package main

import (
	"bytes"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{MinFloor: 1, MaxFloor: 5, InitialFloor: 1, Capacity: 2, TravelTime: 2, StopTime: 3}
}

func mkPassengers(ps ...*Passenger) []*Passenger {
	for i, p := range ps {
		p.seq = i
	}
	return ps
}

func runSim(t *testing.T, cfg Config, ps []*Passenger) *Sim {
	t.Helper()
	if err := cfg.validate(); err != nil {
		t.Fatalf("config: %v", err)
	}
	if err := validatePassengers(cfg, ps); err != nil {
		t.Fatalf("passengers: %v", err)
	}
	s := NewSim(cfg, ps)
	s.Run()
	for _, p := range ps {
		if !p.done {
			t.Fatalf("passenger %s not completed", p.ID)
		}
	}
	if s.maxLoad > cfg.Capacity {
		t.Fatalf("max load %d exceeds capacity %d", s.maxLoad, cfg.Capacity)
	}
	return s
}

// 端点反向：电梯到顶后反向接同层下行乘客。
func TestEndpointReversal(t *testing.T) {
	cfg := testConfig()
	ps := mkPassengers(
		&Passenger{ID: "up", Arrive: 0, From: 1, To: 5},
		&Passenger{ID: "down", Arrive: 1, From: 5, To: 1},
	)
	s := runSim(t, cfg, ps)
	up, down := ps[0], ps[1]
	if up.Alight != 0+3+4*2 { // 停靠3 + 4层运行
		t.Fatalf("up alight = %d, want 11", up.Alight)
	}
	if down.Board != up.Alight {
		t.Fatalf("down should board at top after reversal: board=%d alight=%d", down.Board, up.Alight)
	}
	if down.Alight != down.Board+3+4*2 {
		t.Fatalf("down alight = %d", down.Alight)
	}
	joined := strings.Join(s.logs, "\n")
	if !strings.Contains(joined, "反向") {
		t.Fatalf("expected reversal log, got:\n%s", joined)
	}
}

// 满载滞留：容量 1，两名同层同向乘客，第二名须等下一轮。
func TestFullLoadStranded(t *testing.T) {
	cfg := testConfig()
	cfg.Capacity = 1
	ps := mkPassengers(
		&Passenger{ID: "first", Arrive: 0, From: 1, To: 3},
		&Passenger{ID: "second", Arrive: 0, From: 1, To: 3},
	)
	s := runSim(t, cfg, ps)
	first, second := ps[0], ps[1]
	if first.Board != 0 {
		t.Fatalf("first board = %d, want 0", first.Board)
	}
	if second.Board <= first.Alight {
		t.Fatalf("second must wait for next trip: board=%d first alight=%d", second.Board, first.Alight)
	}
	if s.fullRejects == 0 {
		t.Fatalf("expected full-load rejections")
	}
	if s.maxLoad != 1 {
		t.Fatalf("max load = %d, want 1", s.maxLoad)
	}
}

// 同刻到达：按输入次序登记与上车，先登记再处理到站开门。
func TestSameTimeArrivalOrder(t *testing.T) {
	cfg := testConfig()
	cfg.Capacity = 1
	ps := mkPassengers(
		&Passenger{ID: "b", Arrive: 5, From: 1, To: 2},
		&Passenger{ID: "a", Arrive: 5, From: 1, To: 2},
	)
	s := runSim(t, cfg, ps)
	if ps[0].Board != 5 || ps[1].Board == 5 {
		t.Fatalf("input order must decide boarding: b=%d a=%d", ps[0].Board, ps[1].Board)
	}
	logs := s.logs
	regIdx, boardIdx := -1, -1
	for i, l := range logs {
		if strings.Contains(l, "乘客 b 到达") {
			regIdx = i
		}
		if strings.Contains(l, "乘客 b 在 1 层上车") {
			boardIdx = i
		}
	}
	if regIdx < 0 || boardIdx < 0 || regIdx > boardIdx {
		t.Fatalf("registration must precede boarding:\n%s", strings.Join(logs, "\n"))
	}
}

// 非法记录整体拒绝：重复 ID、起终点相同、越界楼层、非法配置。
func TestInvalidInputRejected(t *testing.T) {
	cfg := testConfig()
	cases := []string{
		`{"config":{"minFloor":1,"maxFloor":5,"initialFloor":1,"capacity":1,"travelTimePerFloor":2,"stopTime":3},"passengers":[{"id":"x","arriveTime":0,"from":1,"to":2},{"id":"x","arriveTime":1,"from":1,"to":3}]}`,
		`{"config":{"minFloor":1,"maxFloor":5,"initialFloor":1,"capacity":1,"travelTimePerFloor":2,"stopTime":3},"passengers":[{"id":"x","arriveTime":0,"from":2,"to":2}]}`,
		`{"config":{"minFloor":1,"maxFloor":5,"initialFloor":1,"capacity":1,"travelTimePerFloor":2,"stopTime":3},"passengers":[{"id":"x","arriveTime":0,"from":1,"to":9}]}`,
		`{"config":{"minFloor":1,"maxFloor":5,"initialFloor":1,"capacity":0,"travelTimePerFloor":2,"stopTime":3},"passengers":[]}`,
		`{"config":{"minFloor":1,"maxFloor":5,"initialFloor":1,"capacity":1,"travelTimePerFloor":0,"stopTime":3},"passengers":[]}`,
	}
	for i, c := range cases {
		var out bytes.Buffer
		if err := run(nil, strings.NewReader(c), &out); err == nil {
			t.Fatalf("case %d: expected rejection, got success", i)
		}
	}
	_ = cfg
}
