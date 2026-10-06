package farecap

import (
	"testing"
)

func rideEvent(id, card, board, alight string, from, to int) Event {
	return Event{Type: TypeRide, ID: id, Card: card, Board: board, Alight: alight, FromZone: from, ToZone: to}
}

func revokeEvent(id, card, target string) Event {
	return Event{Type: TypeRevoke, ID: id, Card: card, TargetID: target}
}

func cardTotal(t *testing.T, store *Store, card string) int {
	t.Helper()
	results, err := store.Results()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Card == card {
			return r.Charged
		}
	}
	return 0
}

func TestDuplicateAndConflict(t *testing.T) {
	store := NewStore(testConfig(), nil)
	r := store.Apply(rideEvent("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1), 1)
	if r.Status != StatusAccepted {
		t.Fatalf("first apply: %s %s", r.Status, r.Reason)
	}
	if r := store.Apply(rideEvent("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1), 2); r.Status != StatusDuplicate {
		t.Fatalf("identical re-submit: %s, want duplicate", r.Status)
	}
	// Conflicting content with same id: rejected and does not alter old record.
	conf := store.Apply(rideEvent("r1", "c1", "2026-10-05T09:00:00", "2026-10-05T09:20:00", 1, 2), 3)
	if conf.Status != StatusRejected {
		t.Fatalf("conflict: %s, want rejected", conf.Status)
	}
	if got := cardTotal(t, store, "c1"); got != 200 {
		t.Fatalf("total after conflict = %d, want 200", got)
	}
}

func TestOverlapRejected(t *testing.T) {
	store := NewStore(testConfig(), nil)
	store.Apply(rideEvent("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:30:00", 1, 1), 1)
	overlap := store.Apply(rideEvent("r2", "c1", "2026-10-05T08:20:00", "2026-10-05T08:50:00", 1, 1), 2)
	if overlap.Status != StatusRejected {
		t.Fatalf("overlapping ride: %s, want rejected", overlap.Status)
	}
	// Touching intervals are allowed (half-open).
	touch := store.Apply(rideEvent("r3", "c1", "2026-10-05T08:30:00", "2026-10-05T08:50:00", 1, 1), 3)
	if touch.Status != StatusAccepted {
		t.Fatalf("touching ride: %s %s, want accepted", touch.Status, touch.Reason)
	}
	// Different cards never overlap-reject.
	other := store.Apply(rideEvent("r4", "c2", "2026-10-05T08:10:00", "2026-10-05T08:20:00", 1, 1), 4)
	if other.Status != StatusAccepted {
		t.Fatalf("other card ride: %s, want accepted", other.Status)
	}
}

func TestRevokeRecomputes(t *testing.T) {
	store := NewStore(testConfig(), nil)
	store.Apply(rideEvent("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1), 1)
	store.Apply(rideEvent("r2", "c1", "2026-10-05T08:40:00", "2026-10-05T09:00:00", 1, 3), 2)
	// Group width 3 -> fare 400.
	if got := cardTotal(t, store, "c1"); got != 400 {
		t.Fatalf("before revoke = %d, want 400", got)
	}
	rv := store.Apply(revokeEvent("v1", "c1", "r2"), 3)
	if rv.Status != StatusRevoked {
		t.Fatalf("revoke: %s %s", rv.Status, rv.Reason)
	}
	// Full recompute from effective records: only r1 @200.
	if got := cardTotal(t, store, "c1"); got != 200 {
		t.Fatalf("after revoke = %d, want 200", got)
	}
	// Revocation history retained.
	if len(store.State().Revokes) != 1 || store.State().Revokes[0].TargetID != "r2" {
		t.Fatalf("revocation history not retained: %+v", store.State().Revokes)
	}
	// Revoking an unknown target is rejected.
	if r := store.Apply(revokeEvent("v2", "c1", "nope"), 4); r.Status != StatusRejected {
		t.Fatalf("unknown target revoke: %s, want rejected", r.Status)
	}
}

func TestCapThenExtendThenRevoke(t *testing.T) {
	cfg := testConfig()
	store := NewStore(cfg, nil)
	// Reach the daily cap with two groups.
	store.Apply(rideEvent("a1", "c1", "2026-10-05T06:00:00", "2026-10-05T06:20:00", 1, 3), 1) // 400
	store.Apply(rideEvent("a2", "c1", "2026-10-05T09:00:00", "2026-10-05T09:20:00", 1, 2), 2) // 300 -> day capped at 500
	if got := cardTotal(t, store, "c1"); got != 500 {
		t.Fatalf("capped total = %d, want 500", got)
	}
	// Extend the second group width 2 -> 3: fare 400, delta 100, but cap -> 0.
	ext := store.Apply(rideEvent("a3", "c1", "2026-10-05T10:00:00", "2026-10-05T10:20:00", 3, 3), 3)
	if ext.Status != StatusAccepted {
		t.Fatalf("extend ride: %s %s", ext.Status, ext.Reason)
	}
	if got := cardTotal(t, store, "c1"); got != 500 {
		t.Fatalf("total after capped extension = %d, want 500", got)
	}
	// Revoke the first ride: recompute -> a2+a3 group fare 400 only.
	store.Apply(revokeEvent("v1", "c1", "a1"), 4)
	if got := cardTotal(t, store, "c1"); got != 400 {
		t.Fatalf("total after revoke = %d, want 400", got)
	}
}

func TestCrossWeekRevoke(t *testing.T) {
	store := NewStore(testConfig(), nil)
	// Sunday ride (service week of Sep 28) and Monday ride (week of Oct 5).
	store.Apply(rideEvent("sun", "c1", "2026-10-04T10:00:00", "2026-10-04T10:20:00", 1, 1), 1)
	store.Apply(rideEvent("mon", "c1", "2026-10-05T10:00:00", "2026-10-05T10:20:00", 1, 1), 2)
	results, err := store.Results()
	if err != nil {
		t.Fatal(err)
	}
	var res *CardResult
	for i := range results {
		if results[i].Card == "c1" {
			res = &results[i]
		}
	}
	if len(res.Weeks) != 2 {
		t.Fatalf("weeks = %d, want 2", len(res.Weeks))
	}
	store.Apply(revokeEvent("v1", "c1", "sun"), 3)
	results, _ = store.Results()
	for i := range results {
		if results[i].Card == "c1" {
			res = &results[i]
		}
	}
	if len(res.Weeks) != 1 || res.Weeks[0].Key != "2026-10-05" {
		t.Fatalf("weeks after revoke = %+v, want only 2026-10-05", res.Weeks)
	}
	if res.Charged != 200 {
		t.Fatalf("charged = %d, want 200", res.Charged)
	}
}

func TestMissingFareAtIngest(t *testing.T) {
	store := NewStore(testConfig(), nil)
	r := store.Apply(rideEvent("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 6), 1)
	if r.Status != StatusRejected {
		t.Fatalf("missing-fare ride: %s, want rejected", r.Status)
	}
}
