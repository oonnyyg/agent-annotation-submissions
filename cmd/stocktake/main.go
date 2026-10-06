package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"example.com/stocktake-adjustment-desk/internal/domain"
	"example.com/stocktake-adjustment-desk/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	if err := run(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: stocktake <command> [flags]

commands:
  inbound    --dir DIR --loc L --sku S --qty N [--memo M]
  outbound   --dir DIR --loc L --sku S --qty N [--memo M]
  query      --dir DIR [--loc L] [--sku S]
  start      --dir DIR --id ID --loc L
  count      --dir DIR --id ID --record RID --loc L --sku S --qty N
  revoke     --dir DIR --id ID --record RID
  complete   --dir DIR --id ID --loc L
  seal       --dir DIR --id ID
  post       --dir DIR --id ID
  show       --dir DIR --id ID
  list       --dir DIR`)
}

func newFS() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func openSvc(dir string) (*domain.Service, func(), error) {
	st, err := store.Open(dir)
	if err != nil {
		return nil, nil, err
	}
	svc, err := domain.NewService(st, filepath.Join(dir, "events.jsonl"))
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	return svc, func() { st.Close() }, nil
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func run(cmd string, args []string) error {
	switch cmd {
	case "inbound", "outbound":
		return movement(cmd, args)
	case "query":
		return query(args)
	case "start":
		return start(args)
	case "count":
		return count(args)
	case "revoke":
		return revoke(args)
	case "complete":
		return complete(args)
	case "seal":
		return seal(args)
	case "post":
		return post(args)
	case "show":
		return show(args)
	case "list":
		return list(args)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func movement(kind string, args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	loc := fs.String("loc", "", "location")
	sku := fs.String("sku", "", "sku")
	qty := fs.Int64("qty", 0, "quantity (non-negative)")
	memo := fs.String("memo", "", "note / source document")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	if kind == "inbound" {
		err = svc.Inbound(*loc, *sku, *qty, *memo)
	} else {
		err = svc.Outbound(*loc, *sku, *qty, *memo)
	}
	if err != nil {
		return err
	}
	printJSON(map[string]any{
		"ok": true, "type": kind, "location": *loc, "sku": *sku, "qty": *qty,
		"current_qty": svc.State().Inventory[*loc][*sku],
	})
	return nil
}

func query(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	loc := fs.String("loc", "", "filter by location")
	sku := fs.String("sku", "", "filter by sku")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	type row struct {
		Location string `json:"location"`
		SKU      string `json:"sku"`
		Qty      int64  `json:"qty"`
	}
	var rows []row
	inv := svc.State().Inventory
	locs := make([]string, 0)
	for l := range inv {
		if *loc == "" || l == *loc {
			locs = append(locs, l)
		}
	}
	sort.Strings(locs)
	for _, l := range locs {
		skus := make([]string, 0)
		for s := range inv[l] {
			if *sku == "" || s == *sku {
				skus = append(skus, s)
			}
		}
		sort.Strings(skus)
		for _, s := range skus {
			rows = append(rows, row{Location: l, SKU: s, Qty: inv[l][s]})
		}
	}
	if rows == nil {
		rows = []row{}
	}
	printJSON(rows)
	return nil
}

func start(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	id := fs.String("id", "", "stocktake id")
	loc := fs.String("loc", "", "location")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := svc.Start(*id, *loc); err != nil {
		return err
	}
	printJSON(map[string]any{"ok": true, "id": *id, "location": *loc,
		"baseline": svc.State().Stocktakes[*id].Baseline})
	return nil
}

func count(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	id := fs.String("id", "", "stocktake id")
	rid := fs.String("record", "", "unique record id")
	loc := fs.String("loc", "", "location")
	sku := fs.String("sku", "", "sku")
	qty := fs.Int64("qty", 0, "counted quantity at freeze time (non-negative)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := svc.Count(*id, *rid, *loc, *sku, *qty); err != nil {
		return err
	}
	printJSON(map[string]any{"ok": true, "stocktake": *id, "record": *rid,
		"counted_total_for_sku": domain.CountedQty(svc.State().Stocktakes[*id], *sku)})
	return nil
}

func revoke(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	id := fs.String("id", "", "stocktake id")
	rid := fs.String("record", "", "record id to revoke")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := svc.Revoke(*id, *rid); err != nil {
		return err
	}
	printJSON(map[string]any{"ok": true, "stocktake": *id, "revoked": *rid})
	return nil
}

func complete(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	id := fs.String("id", "", "stocktake id")
	loc := fs.String("loc", "", "location")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := svc.Complete(*id, *loc); err != nil {
		return err
	}
	printJSON(map[string]any{"ok": true, "stocktake": *id, "location": *loc})
	return nil
}

func seal(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	id := fs.String("id", "", "stocktake id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	details, diff, err := svc.Seal(*id)
	if err != nil {
		return err
	}
	printJSON(map[string]any{"ok": true, "stocktake": *id, "status": "sealed",
		"details": details, "diff": diff})
	return nil
}

func post(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	id := fs.String("id", "", "stocktake id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	diff, reused, err := svc.Post(*id)
	if err != nil {
		return err
	}
	printJSON(map[string]any{"ok": true, "stocktake": *id, "status": "posted",
		"diff": diff, "already_posted": reused})
	return nil
}

func stocktakeJSON(st *domain.Stocktake) map[string]any {
	type rec struct {
		ID       string `json:"id"`
		Location string `json:"location"`
		SKU      string `json:"sku"`
		Qty      int64  `json:"qty"`
		Revoked  bool   `json:"revoked"`
	}
	recs := make([]rec, 0, len(st.Records))
	for _, r := range st.Records {
		recs = append(recs, rec{r.ID, r.Location, r.SKU, r.Qty, r.Revoked})
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	return map[string]any{
		"id": st.ID, "location": st.Location, "status": st.Status,
		"baseline":  st.Baseline,
		"records":   recs,
		"completed": st.Completed,
		"details":   st.Details,
		"diff":      st.Diff,
		"posted":    st.Posted,
	}
}

func show(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	id := fs.String("id", "", "stocktake id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	st := svc.State().Stocktakes[*id]
	if st == nil {
		return fmt.Errorf("stocktake %q not found", *id)
	}
	printJSON(stocktakeJSON(st))
	return nil
}

func list(args []string) error {
	fs := newFS()
	dir := fs.String("dir", "", "data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, closeFn, err := openSvc(*dir)
	if err != nil {
		return err
	}
	defer closeFn()
	ids := make([]string, 0)
	for id := range svc.State().Stocktakes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, stocktakeJSON(svc.State().Stocktakes[id]))
	}
	printJSON(out)
	return nil
}
