package domain

import (
	"path/filepath"
	"testing"

	"example.com/stocktake-adjustment-desk/internal/store"
)

func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := NewService(st, filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return svc, dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", sub)
	}
	if !contains(err.Error(), sub) {
		t.Fatalf("error %q does not contain %q", err.Error(), sub)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Mid-stocktake outbound keeps moving current stock but never moves the
// frozen baseline; posting adds only the difference.
func TestMidCountOutbound(t *testing.T) {
	svc, _ := newTestService(t)
	const loc = "L1"
	must(t, svc.Inbound(loc, "A", 10, ""))
	must(t, svc.Inbound(loc, "B", 5, ""))
	must(t, svc.Start("ST1", loc))

	// Movement during counting changes current stock, not the baseline.
	must(t, svc.Outbound(loc, "A", 4, "order-1"))
	if got := svc.State().Inventory[loc]["A"]; got != 6 {
		t.Fatalf("current A = %d, want 6", got)
	}
	if base := svc.State().Stocktakes["ST1"].Baseline["A"]; base != 10 {
		t.Fatalf("frozen baseline A = %d, want 10", base)
	}

	// Counts express the freeze-time truth: A=10 (unchanged), B=7 (gain).
	must(t, svc.Count("ST1", "r1", loc, "A", 10))
	must(t, svc.Count("ST1", "r2", loc, "B", 7))
	must(t, svc.Complete("ST1", loc))
	_, diff, err := svc.Seal("ST1")
	must(t, err)
	if diff["B"] != 2 || diff["A"] != 0 {
		t.Fatalf("diff = %v, want A:0 B:+2", diff)
	}
	_, _, err = svc.Post("ST1")
	must(t, err)
	// Current A stayed 6 (baseline matched), B became 7.
	if got := svc.State().Inventory[loc]["A"]; got != 6 {
		t.Fatalf("A after post = %d, want 6", got)
	}
	if got := svc.State().Inventory[loc]["B"]; got != 7 {
		t.Fatalf("B after post = %d, want 7", got)
	}
}

// A baseline sku with no surviving count record is treated as counted zero.
func TestMissedSkuZeroed(t *testing.T) {
	svc, _ := newTestService(t)
	const loc = "L1"
	must(t, svc.Inbound(loc, "A", 10, ""))
	must(t, svc.Inbound(loc, "B", 8, ""))
	must(t, svc.Start("ST1", loc))
	must(t, svc.Count("ST1", "r1", loc, "A", 9))
	// B is never counted.
	must(t, svc.Complete("ST1", loc))
	details, diff, err := svc.Seal("ST1")
	must(t, err)
	if diff["A"] != -1 || diff["B"] != -8 {
		t.Fatalf("diff = %v, want A:-1 B:-8", diff)
	}
	detailB := false
	for _, d := range details {
		if d.SKU == "B" && d.Baseline == 8 && d.Counted == 0 && d.Diff == -8 {
			detailB = true
		}
	}
	if !detailB {
		t.Fatalf("sealed details missing zero-count line for B: %+v", details)
	}
}

// If posting would drive a sku negative, the whole posting is rejected;
// after inbound restores room, the retained seal can be retried successfully.
func TestPostRejectedThenRetried(t *testing.T) {
	svc, _ := newTestService(t)
	const loc = "L1"
	must(t, svc.Inbound(loc, "A", 10, ""))
	must(t, svc.Start("ST1", loc))
	must(t, svc.Count("ST1", "r1", loc, "A", 10))
	must(t, svc.Complete("ST1", loc))
	_, _, err := svc.Seal("ST1")
	must(t, err)

	// New stocktake ST2 establishes a -6 adjustment, then stock drains to 3.
	must(t, svc.Start("ST2", loc))
	must(t, svc.Count("ST2", "r2", loc, "A", 4))
	must(t, svc.Complete("ST2", loc))
	_, diff2, err := svc.Seal("ST2")
	must(t, err)
	if diff2["A"] != -6 {
		t.Fatalf("ST2 diff = %v, want -6", diff2)
	}
	must(t, svc.Outbound(loc, "A", 7, "order-2")) // current 10 -> 3
	_, _, err = svc.Post("ST2")
	wantErr(t, err, "negative")
	if svc.State().Stocktakes["ST2"].Status != "sealed" {
		t.Fatalf("ST2 status = %s, want sealed retained for retry",
			svc.State().Stocktakes["ST2"].Status)
	}

	// Inbound makes room; retry applies the same frozen diff.
	must(t, svc.Inbound(loc, "A", 5, "top-up")) // 3 -> 8
	posted, reused, err := svc.Post("ST2")
	must(t, err)
	if reused || posted["A"] != -6 {
		t.Fatalf("retry post = %v reused=%v, want -6/false", posted, reused)
	}
	if got := svc.State().Inventory[loc]["A"]; got != 2 {
		t.Fatalf("A after retry = %d, want 2", got)
	}

	// Posting again returns the original result and changes nothing.
	posted2, reused2, err := svc.Post("ST2")
	must(t, err)
	if !reused2 || posted2["A"] != -6 {
		t.Fatalf("second post = %v reused=%v", posted2, reused2)
	}
	if got := svc.State().Inventory[loc]["A"]; got != 2 {
		t.Fatalf("A changed on repeated post = %d, want 2", got)
	}
}

func TestRecordRules(t *testing.T) {
	svc, _ := newTestService(t)
	const loc = "L1"
	must(t, svc.Inbound(loc, "A", 10, ""))
	must(t, svc.Start("ST1", loc))

	// Different ids sum, no auto dedupe by sku.
	must(t, svc.Count("ST1", "r1", loc, "A", 6))
	must(t, svc.Count("ST1", "r2", loc, "A", 4))
	if got := CountedQty(svc.State().Stocktakes["ST1"], "A"); got != 10 {
		t.Fatalf("counted A = %d, want 10", got)
	}
	// Same id, same content: idempotent.
	must(t, svc.Count("ST1", "r1", loc, "A", 6))
	if got := CountedQty(svc.State().Stocktakes["ST1"], "A"); got != 10 {
		t.Fatalf("counted A after re-import = %d, want 10", got)
	}
	// Same id, different content: rejected.
	wantErr(t, svc.Count("ST1", "r1", loc, "A", 7), "different content")

	// Revoke removes that record's contribution and is idempotent.
	must(t, svc.Revoke("ST1", "r1"))
	if got := CountedQty(svc.State().Stocktakes["ST1"], "A"); got != 4 {
		t.Fatalf("counted A after revoke = %d, want 4", got)
	}
	must(t, svc.Revoke("ST1", "r1"))

	// Nothing may be edited after sealing.
	must(t, svc.Complete("ST1", loc))
	_, _, err := svc.Seal("ST1")
	must(t, err)
	wantErr(t, svc.Count("ST1", "r3", loc, "A", 1), "sealed")
	wantErr(t, svc.Revoke("ST1", "r2"), "sealed")
	_, _, err = svc.Seal("ST1")
	wantErr(t, err, "already")
}

func TestInventoryAndLocationGuards(t *testing.T) {
	svc, _ := newTestService(t)
	const loc = "L1"
	must(t, svc.Inbound(loc, "A", 3, ""))
	wantErr(t, svc.Outbound(loc, "A", 4, ""), "rejected")
	wantErr(t, svc.Inbound(loc, "A", -1, ""), "non-negative")

	must(t, svc.Start("ST1", loc))
	wantErr(t, svc.Start("ST2", loc), "unfinished")
	must(t, svc.Inbound("L2", "X", 1, ""))
	must(t, svc.Start("ST3", "L2")) // another location is fine

	// Seal requires explicit completion.
	_, _, err := svc.Seal("ST1")
	wantErr(t, err, "not marked complete")

	// Counts must target the stocktake's own location.
	wantErr(t, svc.Count("ST1", "r9", "L2", "A", 1), "does not match")

	// Once ST1 is sealed, its location is free for a new stocktake.
	must(t, svc.Complete("ST3", "L2"))
	_, _, err = svc.Seal("ST3")
	must(t, err)
	must(t, svc.Complete("ST1", loc))
	_, _, err = svc.Seal("ST1")
	must(t, err)
	must(t, svc.Start("ST4", loc))
}

// State survives a process restart: reopening replays the JSONL log.
func TestRestartReplaysLog(t *testing.T) {
	svc, dir := newTestService(t)
	const loc = "L1"
	must(t, svc.Inbound(loc, "A", 10, ""))
	must(t, svc.Start("ST1", loc))
	must(t, svc.Count("ST1", "r1", loc, "A", 7))
	must(t, svc.Complete("ST1", loc))
	_, _, err := svc.Seal("ST1")
	must(t, err)

	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc2, err := NewService(st, filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := svc2.State().Inventory[loc]["A"]; got != 10 {
		t.Fatalf("current A after restart = %d, want 10", got)
	}
	st1 := svc2.State().Stocktakes["ST1"]
	if st1.Status != "sealed" || st1.Diff["A"] != -3 {
		t.Fatalf("ST1 after restart = %+v diff %v", st1.Status, st1.Diff)
	}
	// Sealed result can be posted after restart.
	_, _, err = svc2.Post("ST1")
	must(t, err)
	if got := svc2.State().Inventory[loc]["A"]; got != 7 {
		t.Fatalf("A after post on restart = %d, want 7", got)
	}
}
