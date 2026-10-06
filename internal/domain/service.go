package domain

import (
	"fmt"
	"sort"

	"example.com/stocktake-adjustment-desk/internal/store"
)

// Service validates commands against the current state and appends events.
type Service struct {
	store *store.Store
	state *State
}

// NewService builds a service over an opened store and replays history.
func NewService(st *store.Store, path string) (*Service, error) {
	s, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Service{store: st, state: s}, nil
}

// State exposes the materialized state (read-only use expected).
func (svc *Service) State() *State { return svc.state }

func (svc *Service) commit(e *store.Event) error {
	if err := svc.store.Append(e); err != nil {
		return err
	}
	return svc.state.Apply(e)
}

func validateID(kind, v string) error {
	if v == "" {
		return fmt.Errorf("%s must not be empty", kind)
	}
	return nil
}

func validateQty(q int64) error {
	if q < 0 {
		return fmt.Errorf("quantity must be non-negative, got %d", q)
	}
	return nil
}

// Inbound increases current stock at a location.
func (svc *Service) Inbound(loc, sku string, qty int64, memo string) error {
	if err := validateID("location", loc); err != nil {
		return err
	}
	if err := validateID("sku", sku); err != nil {
		return err
	}
	if err := validateQty(qty); err != nil {
		return err
	}
	return svc.commit(&store.Event{
		Type: "inbound", Location: loc, SKU: sku, Qty: qty, Memo: memo,
	})
}

// Outbound decreases current stock; it is rejected if it would go negative.
func (svc *Service) Outbound(loc, sku string, qty int64, memo string) error {
	if err := validateID("location", loc); err != nil {
		return err
	}
	if err := validateID("sku", sku); err != nil {
		return err
	}
	if err := validateQty(qty); err != nil {
		return err
	}
	if svc.state.locQty(loc, sku)-qty < 0 {
		return fmt.Errorf("outbound rejected: %s/%s has %d, cannot take %d",
			loc, sku, svc.state.locQty(loc, sku), qty)
	}
	return svc.commit(&store.Event{
		Type: "outbound", Location: loc, SKU: sku, Qty: qty, Memo: memo,
	})
}

// Start opens a stocktake for one location and freezes its current book qty.
func (svc *Service) Start(id, loc string) error {
	if err := validateID("stocktake id", id); err != nil {
		return err
	}
	if err := validateID("location", loc); err != nil {
		return err
	}
	if _, dup := svc.state.Stocktakes[id]; dup {
		return fmt.Errorf("stocktake %q already exists", id)
	}
	if other, busy := svc.state.activeByLocation[loc]; busy {
		return fmt.Errorf("location %q has an unfinished stocktake %q", loc, other)
	}
	baseline := map[string]int64{}
	for sku, q := range svc.state.Inventory[loc] {
		baseline[sku] = q
	}
	return svc.commit(&store.Event{
		Type: "stocktake_started", Stocktake: id, Location: loc, Baseline: baseline,
	})
}

// Count imports one count record. Re-importing the same record id with the
// same (location, sku, qty) is an idempotent no-op; any content mismatch is
// rejected. Different ids are never deduplicated: their quantities add up.
func (svc *Service) Count(id, recordID, loc, sku string, qty int64) error {
	if err := validateID("record id", recordID); err != nil {
		return err
	}
	if err := validateID("sku", sku); err != nil {
		return err
	}
	if err := validateQty(qty); err != nil {
		return err
	}
	st, err := svc.openStocktake(id)
	if err != nil {
		return err
	}
	if loc != st.Location {
		return fmt.Errorf("record location %q does not match stocktake location %q", loc, st.Location)
	}
	if st.Completed[loc] {
		return fmt.Errorf("location %q already marked complete for %q", loc, id)
	}
	if r, exists := st.Records[recordID]; exists {
		if r.Location != loc || r.SKU != sku || r.Qty != qty {
			return fmt.Errorf("record %q already exists with different content", recordID)
		}
		return nil // identical re-import
	}
	return svc.commit(&store.Event{
		Type:      "count_recorded",
		Stocktake: id,
		RecordID:  recordID,
		Location:  loc,
		SKU:       sku,
		Qty:       qty,
	})
}

// Revoke withdraws a record that has not been sealed.
func (svc *Service) Revoke(id, recordID string) error {
	st, err := svc.openStocktake(id)
	if err != nil {
		return err
	}
	r := st.Records[recordID]
	if r == nil {
		return fmt.Errorf("record %q not found in stocktake %q", recordID, id)
	}
	if r.Revoked {
		return nil // idempotent
	}
	return svc.commit(&store.Event{
		Type: "count_revoked", Stocktake: id, RecordID: recordID, Location: st.Location,
	})
}

// Complete marks a location as fully counted.
func (svc *Service) Complete(id, loc string) error {
	st, err := svc.openStocktake(id)
	if err != nil {
		return err
	}
	if loc != st.Location {
		return fmt.Errorf("stocktake %q is for location %q, not %q", id, st.Location, loc)
	}
	if st.Completed[loc] {
		return nil
	}
	return svc.commit(&store.Event{
		Type: "location_completed", Stocktake: id, Location: loc,
	})
}

func (svc *Service) openStocktake(id string) (*Stocktake, error) {
	st := svc.state.Stocktakes[id]
	if st == nil {
		return nil, fmt.Errorf("stocktake %q not found", id)
	}
	if st.Status != "open" {
		return nil, fmt.Errorf("stocktake %q is %s and can no longer be edited", id, st.Status)
	}
	return st, nil
}

// CountedQty sums the non-revoked records for one sku. Records are never
// auto-deduplicated by sku; each record id contributes independently.
func CountedQty(st *Stocktake, sku string) int64 {
	var total int64
	for _, r := range st.Records {
		if !r.Revoked && r.SKU == sku {
			total += r.Qty
		}
	}
	return total
}

// Seal freezes the count result. The counted total per sku is the union of
// baseline skus and counted skus; baseline skus with no surviving record are
// treated as counted zero.
func (svc *Service) Seal(id string) ([]store.CountDetail, map[string]int64, error) {
	st := svc.state.Stocktakes[id]
	if st == nil {
		return nil, nil, fmt.Errorf("stocktake %q not found", id)
	}
	if st.Status == "sealed" || st.Status == "posted" {
		return nil, nil, fmt.Errorf("stocktake %q is already %s", id, st.Status)
	}
	// A stocktake covers exactly one location; it must be confirmed complete.
	if !st.Completed[st.Location] {
		return nil, nil, fmt.Errorf("location %q not marked complete", st.Location)
	}
	skus := map[string]bool{}
	for sku := range st.Baseline {
		skus[sku] = true
	}
	for _, r := range st.Records {
		if !r.Revoked {
			skus[r.SKU] = true
		}
	}
	skuList := make([]string, 0, len(skus))
	for sku := range skus {
		skuList = append(skuList, sku)
	}
	sort.Strings(skuList)
	details := make([]store.CountDetail, 0, len(skuList))
	diff := map[string]int64{}
	for _, sku := range skuList {
		base := st.Baseline[sku]
		counted := CountedQty(st, sku)
		d := counted - base
		details = append(details, store.CountDetail{
			SKU: sku, Baseline: base, Counted: counted, Diff: d,
		})
		if d != 0 {
			diff[sku] = d
		}
	}
	err := svc.commit(&store.Event{
		Type:      "stocktake_sealed",
		Stocktake: id,
		Location:  st.Location,
		Details:   details,
		Diff:      diff,
	})
	if err != nil {
		return nil, nil, err
	}
	return details, diff, nil
}

// Post applies the frozen diff to current stock. It never replaces stock with
// the counted qty; it adds (counted - baseline). If any sku would become
// negative the whole posting is rejected and the seal stays retryable.
// Posting an already posted stocktake returns the original frozen result.
func (svc *Service) Post(id string) (map[string]int64, bool, error) {
	st := svc.state.Stocktakes[id]
	if st == nil {
		return nil, false, fmt.Errorf("stocktake %q not found", id)
	}
	if st.Status == "posted" {
		return cloneMap(st.Diff), true, nil // idempotent: original result
	}
	if st.Status != "sealed" {
		return nil, false, fmt.Errorf("stocktake %q is not sealed", id)
	}
	for sku, d := range st.Diff {
		if svc.state.locQty(st.Location, sku)+d < 0 {
			return nil, false, fmt.Errorf(
				"post rejected: %s/%s would become negative (%d + %d)",
				st.Location, sku, svc.state.locQty(st.Location, sku), d)
		}
	}
	if err := svc.commit(&store.Event{
		Type: "stocktake_posted", Stocktake: id, Location: st.Location, Diff: st.Diff,
	}); err != nil {
		return nil, false, err
	}
	return cloneMap(st.Diff), false, nil
}
