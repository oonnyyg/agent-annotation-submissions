package farecap

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadReopenConsistency(t *testing.T) {
	cfg := testConfig()
	store := NewStore(cfg, nil)
	lines := []Event{
		rideEvent("r1", "c1", "2026-10-05T08:00:00", "2026-10-05T08:20:00", 1, 1),
		rideEvent("r2", "c1", "2026-10-05T08:40:00", "2026-10-05T09:00:00", 1, 3),
		rideEvent("r3", "c2", "2026-10-06T07:00:00", "2026-10-06T07:20:00", 2, 2),
	}
	for i, e := range lines {
		if r := store.Apply(e, i+1); r.Status != StatusAccepted {
			t.Fatalf("apply %s: %s %s", e.ID, r.Status, r.Reason)
		}
	}
	before, err := store.Results()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveState(path, store.State()); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(cfg, loaded)
	after, err := reopened.Results()
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("reopened card count = %d, want %d", len(after), len(before))
	}
	for i := range before {
		if before[i].Charged != after[i].Charged {
			t.Fatalf("card %s charged %d after reopen, want %d",
				before[i].Card, after[i].Charged, before[i].Charged)
		}
	}

	// Replaying the identical input after reopen must not double-bill.
	for i, e := range lines {
		r := reopened.Apply(e, i+1)
		if r.Status != StatusDuplicate {
			t.Fatalf("replay %s: %s, want duplicate", e.ID, r.Status)
		}
	}
	replay, err := reopened.Results()
	if err != nil {
		t.Fatal(err)
	}
	if replay[0].Charged != before[0].Charged {
		t.Fatalf("replay changed totals: %d vs %d", replay[0].Charged, before[0].Charged)
	}
}
