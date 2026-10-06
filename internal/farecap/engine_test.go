package farecap

import (
	"testing"
	"time"
)

func testConfig() *Config {
	return &Config{
		UTCOffsetSeconds:      8 * 3600,
		ServiceDayStartHour:   4,
		TransferWindowSeconds: 90 * 60,
		Fares: []FareRule{
			{Width: 1, Fare: 200},
			{Width: 2, Fare: 300},
			{Width: 3, Fare: 400},
			{Width: 4, Fare: 500},
		},
		DailyCap:  500,
		WeeklyCap: 1500,
	}
}

func mustRide(t *testing.T, id, card, board, alight string, from, to int, loc *time.Location) Ride {
	t.Helper()
	e := Event{Type: TypeRide, ID: id, Card: card, Board: board, Alight: alight, FromZone: from, ToZone: to}
	r, err := e.ParseRide(loc)
	if err != nil {
		t.Fatalf("parse ride %s: %v", id, err)
	}
	return r
}

func rideTotal(res CardResult) int {
	total := 0
	for _, d := range res.Days {
		total += d.Total
	}
	return total
}

func TestTransferGroupAndCap(t *testing.T) {
	cfg := testConfig()
	clock := NewClock(cfg)
	loc := clock.loc
	// First ride width1 costs 200; second ride within 90min extends width to 3:
	// group fare 400, delta 200; third ride pushes width to 4 (fare 500),
	// delta 100 fills the daily cap exactly; a fourth ride starting a new
	// group contributes 0 because the daily cap is exhausted.
	rides := []Ride{
		mustRide(t, "r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1, loc),
		mustRide(t, "r2", "c1", "2026-10-05T08:40:00", "2026-10-05T09:00:00", 1, 3, loc),
		mustRide(t, "r3", "c1", "2026-10-05T09:10:00", "2026-10-05T09:30:00", 4, 4, loc),
		mustRide(t, "r4", "c1", "2026-10-05T12:00:00", "2026-10-05T12:20:00", 1, 1, loc),
	}
	res, err := Recompute("c1", rides, cfg, clock)
	if err != nil {
		t.Fatal(err)
	}
	if rideTotal(res) != 500 {
		t.Fatalf("total = %d, want 500", rideTotal(res))
	}
	if got := res.Rides[2].Charge; got != 100 {
		t.Fatalf("third ride charge = %d, want 100", got)
	}
	if got := res.Rides[3].Charge; got != 0 {
		t.Fatalf("fourth ride charge = %d, want 0 (daily cap exhausted)", got)
	}
	if len(res.Days) != 1 {
		t.Fatalf("days = %d, want 1", len(res.Days))
	}
	if got := res.Rides[2].GroupID; got != 1 {
		t.Fatalf("group = %d, want 1", got)
	}
	if got := res.Rides[3].GroupID; got != 2 {
		t.Fatalf("fourth ride group = %d, want 2", got)
	}
}

func TestWindowDoesNotExtend(t *testing.T) {
	cfg := testConfig()
	clock := NewClock(cfg)
	loc := clock.loc
	// First at 08:00, second at 09:29 joins (89min < 90); third at 10:58
	// is 178min from first board -> new group even though 89min from second.
	rides := []Ride{
		mustRide(t, "r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1, loc),
		mustRide(t, "r2", "c1", "2026-10-05T09:29:00", "2026-10-05T09:45:00", 1, 1, loc),
		mustRide(t, "r3", "c1", "2026-10-05T10:58:00", "2026-10-05T11:10:00", 1, 1, loc),
	}
	res, err := Recompute("c1", rides, cfg, clock)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rides[2].GroupID != 2 {
		t.Fatalf("third ride group = %d, want 2", res.Rides[2].GroupID)
	}
}

func TestServiceDayBoundary(t *testing.T) {
	cfg := testConfig()
	clock := NewClock(cfg)
	loc := clock.loc
	// 03:50 and 04:10 local on 2026-10-05 are in different service days
	// (the 03:50 ride belongs to the service day starting 10-04 04:00).
	rides := []Ride{
		mustRide(t, "r1", "c1", "2026-10-05T03:50:00", "2026-10-05T04:05:00", 1, 1, loc),
		mustRide(t, "r2", "c1", "2026-10-05T04:10:00", "2026-10-05T04:30:00", 1, 1, loc),
	}
	res, err := Recompute("c1", rides, cfg, clock)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Days) != 2 {
		t.Fatalf("days = %d, want 2; keys=%v", len(res.Days), dayKeys(res))
	}
	if res.Rides[0].GroupID == res.Rides[1].GroupID {
		t.Fatal("rides across service days must not share a transfer group")
	}
	if res.Days[0].Key != "2026-10-04" || res.Days[1].Key != "2026-10-05" {
		t.Fatalf("unexpected day keys: %s %s", res.Days[0].Key, res.Days[1].Key)
	}
}

func dayKeys(res CardResult) []string {
	keys := []string{}
	for _, d := range res.Days {
		keys = append(keys, d.Key)
	}
	return keys
}

func TestServiceWeekBoundary(t *testing.T) {
	cfg := testConfig()
	clock := NewClock(cfg)
	// Sunday 2026-10-04 and Monday 2026-10-05 are in different service weeks.
	if got := clock.WeekKey(mustParse(t, "2026-10-04T12:00:00", clock.loc)); got != "2026-09-28" {
		t.Fatalf("sunday week key = %s, want 2026-09-28", got)
	}
	if got := clock.WeekKey(mustParse(t, "2026-10-05T03:00:00", clock.loc)); got != "2026-09-28" {
		t.Fatalf("monday 03:00 (before 04:00) week key = %s, want 2026-09-28", got)
	}
	if got := clock.WeekKey(mustParse(t, "2026-10-05T05:00:00", clock.loc)); got != "2026-10-05" {
		t.Fatalf("monday 05:00 week key = %s, want 2026-10-05", got)
	}
}

func mustParse(t *testing.T, s string, loc *time.Location) time.Time {
	t.Helper()
	e := Event{Type: TypeRide, ID: "x", Card: "c", Board: s, Alight: s, FromZone: 1, ToZone: 1}
	r, err := e.ParseRide(loc)
	if err != nil {
		t.Fatal(err)
	}
	return r.Board
}

func TestWeeklyCapAcrossDays(t *testing.T) {
	cfg := testConfig()
	clock := NewClock(cfg)
	loc := clock.loc
	// Mon-Thu (Oct 5-8, same service week): two separate groups per day at
	// 10:00 and 13:00 (>90min apart). 8 rides x 200 = 1600 nominal, daily
	// totals 400 stay under 500, weekly cap 1500 limits the total.
	var rides []Ride
	n := 0
	slots := [][2]string{{"10:00:00", "10:20:00"}, {"13:00:00", "13:20:00"}}
	for d := 5; d <= 8; d++ {
		for _, hm := range slots {
			n++
			rides = append(rides, mustRide(t,
				"r"+itoa(n), "c1",
				"2026-10-"+two(d)+"T"+hm[0],
				"2026-10-"+two(d)+"T"+hm[1], 1, 1, loc))
		}
	}
	res, err := Recompute("c1", rides, cfg, clock)
	if err != nil {
		t.Fatal(err)
	}
	if rideTotal(res) != 1500 {
		t.Fatalf("total = %d, want 1500 (weekly cap)", rideTotal(res))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func two(n int) string {
	s := itoa(n)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func TestMissingFareRejected(t *testing.T) {
	cfg := testConfig()
	clock := NewClock(cfg)
	loc := clock.loc
	rides := []Ride{
		mustRide(t, "r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 6, loc),
	}
	if _, err := Recompute("c1", rides, cfg, clock); err == nil {
		t.Fatal("expected error for width 6 with no configured fare")
	}
}
