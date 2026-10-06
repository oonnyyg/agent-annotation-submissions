package desk

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type TakeStartEvent struct {
	TakeID    string   `json:"take_id"`
	Locations []string `json:"locations"`
}

type CountRecordEvent struct {
	TakeID string `json:"take_id"`
	RecID  string `json:"rec_id"`
	Loc    string `json:"loc"`
	SKU    string `json:"sku"`
	Qty    int    `json:"qty"`
}

type CountVoidEvent struct {
	TakeID string `json:"take_id"`
	Loc    string `json:"loc"`
	RecID  string `json:"rec_id"`
}

type CountCompleteEvent struct {
	TakeID string `json:"take_id"`
	Loc    string `json:"loc"`
}

type TakeRefEvent struct {
	TakeID string `json:"take_id"`
}

// startTake 冻结所选库位的账面数量作为基线。
func (s *State) startTake(d TakeStartEvent) error {
	if d.TakeID == "" || len(d.Locations) == 0 {
		return errors.New("盘点编号与库位不能为空")
	}
	if _, ok := s.Takes[d.TakeID]; ok {
		return fmt.Errorf("盘点 %s 已存在", d.TakeID)
	}
	seen := map[string]bool{}
	for _, loc := range d.Locations {
		if seen[loc] {
			return fmt.Errorf("库位 %s 重复", loc)
		}
		seen[loc] = true
		for _, t := range s.Takes {
			if t.Status != TakePosted && t.hasLoc(loc) {
				return fmt.Errorf("库位 %s 已属于未结束盘点 %s", loc, t.ID)
			}
		}
	}
	baseline := map[string]map[string]int{}
	for _, loc := range d.Locations {
		m := map[string]int{}
		for sku, qty := range s.Stock[loc] {
			m[sku] = qty
		}
		baseline[loc] = m
	}
	s.Takes[d.TakeID] = &Take{
		ID:        d.TakeID,
		Locations: append([]string(nil), d.Locations...),
		Status:    TakeOpen,
		Baseline:  baseline,
		Records:   map[string]*CountRecord{},
		Completed: map[string]bool{},
	}
	return nil
}

func (s *State) openTake(id string) (*Take, error) {
	t, ok := s.Takes[id]
	if !ok {
		return nil, fmt.Errorf("盘点 %s 不存在", id)
	}
	if t.Status == TakePosted {
		return nil, fmt.Errorf("盘点 %s 已入账，不可再修改", id)
	}
	if t.Status == TakeSealed {
		return nil, fmt.Errorf("盘点 %s 已封存，请先重开", id)
	}
	return t, nil
}

// addRecord 导入清点记录：同编号同内容幂等，冲突拒绝。
func (s *State) addRecord(d CountRecordEvent) error {
	t, err := s.openTake(d.TakeID)
	if err != nil {
		return err
	}
	if !t.hasLoc(d.Loc) {
		return fmt.Errorf("库位 %s 不在盘点 %s 范围内", d.Loc, d.TakeID)
	}
	if d.Qty < 0 {
		return errors.New("清点数量必须为非负整数")
	}
	key := d.Loc + "/" + d.RecID
	if old, ok := t.Records[key]; ok {
		if old.SKU == d.SKU && old.Qty == d.Qty && !old.Void {
			return nil // 重复导入，幂等
		}
		return fmt.Errorf("记录编号 %s 冲突: 已有 %s=%d", d.RecID, old.SKU, old.Qty)
	}
	t.Records[key] = &CountRecord{RecID: d.RecID, Loc: d.Loc, SKU: d.SKU, Qty: d.Qty}
	return nil
}

func (s *State) voidRecord(d CountVoidEvent) error {
	t, err := s.openTake(d.TakeID)
	if err != nil {
		return err
	}
	r, ok := t.Records[d.Loc+"/"+d.RecID]
	if !ok {
		return fmt.Errorf("记录 %s 不存在", d.RecID)
	}
	if r.Void {
		return fmt.Errorf("记录 %s 已撤销", d.RecID)
	}
	r.Void = true
	return nil
}

func (s *State) completeLoc(d CountCompleteEvent) error {
	t, err := s.openTake(d.TakeID)
	if err != nil {
		return err
	}
	if !t.hasLoc(d.Loc) {
		return fmt.Errorf("库位 %s 不在盘点 %s 范围内", d.Loc, d.TakeID)
	}
	t.Completed[d.Loc] = true
	return nil
}

// countedByLocSKU 汇总某库位有效清点记录（不同编号数量相加）。
func (t *Take) countedByLocSKU() map[string]map[string]int {
	out := map[string]map[string]int{}
	for _, r := range t.Records {
		if r.Void {
			continue
		}
		m, ok := out[r.Loc]
		if !ok {
			m = map[string]int{}
			out[r.Loc] = m
		}
		m[r.SKU] += r.Qty
	}
	return out
}

// seal 在全部库位确认完成后冻结差异明细。
func (s *State) seal(takeID string) (any, error) {
	t, ok := s.Takes[takeID]
	if !ok {
		return nil, fmt.Errorf("盘点 %s 不存在", takeID)
	}
	if t.Status != TakeOpen {
		return nil, fmt.Errorf("盘点 %s 当前状态 %s，不能封存", takeID, t.Status)
	}
	for _, loc := range t.Locations {
		if !t.Completed[loc] {
			return nil, fmt.Errorf("库位 %s 尚未确认清点完成", loc)
		}
	}
	counted := t.countedByLocSKU()
	skus := map[string]bool{}
	for _, loc := range t.Locations {
		for sku := range t.Baseline[loc] {
			skus[loc+"/"+sku] = true
		}
		for sku := range counted[loc] {
			skus[loc+"/"+sku] = true
		}
	}
	keys := make([]string, 0, len(skus))
	for k := range skus {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var diffs []DiffLine
	for _, k := range keys {
		loc, sku, _ := strings.Cut(k, "/")
		b := t.Baseline[loc][sku]
		c := counted[loc][sku]
		diffs = append(diffs, DiffLine{SKU: loc + "/" + sku, Baseline: b, Counted: c, Diff: c - b})
	}
	t.Diffs = diffs
	t.Status = TakeSealed
	return diffs, nil
}

// post 将封存差异整体加到当前库存；任一商品变负则整次拒绝。
func (s *State) post(takeID string, seq int) (any, error) {
	t, ok := s.Takes[takeID]
	if !ok {
		return nil, fmt.Errorf("盘点 %s 不存在", takeID)
	}
	if t.Status == TakePosted {
		return t.Diffs, nil // 重复入账返回原结果
	}
	if t.Status != TakeSealed {
		return nil, fmt.Errorf("盘点 %s 未封存，不能入账", takeID)
	}
	for _, d := range t.Diffs {
		loc, sku, _ := strings.Cut(d.SKU, "/")
		if s.Stock[loc][sku]+d.Diff < 0 {
			return nil, fmt.Errorf("入账将使 %s/%s 库存变负（当前 %d, 差异 %d），整次拒绝", loc, sku, s.Stock[loc][sku], d.Diff)
		}
	}
	for _, d := range t.Diffs {
		loc, sku, _ := strings.Cut(d.SKU, "/")
		s.addStock(loc, sku, d.Diff)
	}
	t.Status = TakePosted
	t.PostSeq = seq
	return t.Diffs, nil
}

// reopen 重开未入账的封存盘点，保留清点记录与封存明细。
func (s *State) reopen(takeID string) error {
	t, ok := s.Takes[takeID]
	if !ok {
		return fmt.Errorf("盘点 %s 不存在", takeID)
	}
	if t.Status == TakePosted {
		return fmt.Errorf("盘点 %s 已入账，不可重开", takeID)
	}
	t.Status = TakeOpen
	return nil
}
