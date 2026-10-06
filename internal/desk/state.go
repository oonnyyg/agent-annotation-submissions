package desk

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

func (s *State) apply(ev Event) error {
	_, err := s.applyResult(ev)
	return err
}

func (s *State) applyResult(ev Event) (any, error) {
	switch ev.Op {
	case "inbound":
		var d MoveEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		if d.Qty < 0 {
			return nil, errors.New("数量必须为非负整数")
		}
		s.addStock(d.Loc, d.SKU, d.Qty)
		return s.Stock[d.Loc][d.SKU], nil
	case "outbound":
		var d MoveEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		if d.Qty < 0 {
			return nil, errors.New("数量必须为非负整数")
		}
		if s.Stock[d.Loc][d.SKU] < d.Qty {
			return nil, fmt.Errorf("出库后库存为负: %s/%s 现有 %d, 需出 %d", d.Loc, d.SKU, s.Stock[d.Loc][d.SKU], d.Qty)
		}
		s.addStock(d.Loc, d.SKU, -d.Qty)
		return s.Stock[d.Loc][d.SKU], nil
	case "take_start":
		var d TakeStartEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		return nil, s.startTake(d)
	case "count_record":
		var d CountRecordEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		return nil, s.addRecord(d)
	case "count_void":
		var d CountVoidEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		return nil, s.voidRecord(d)
	case "count_complete":
		var d CountCompleteEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		return nil, s.completeLoc(d)
	case "take_seal":
		var d TakeRefEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		return s.seal(d.TakeID)
	case "take_post":
		var d TakeRefEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		return s.post(d.TakeID, ev.Seq)
	case "take_reopen":
		var d TakeRefEvent
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return nil, err
		}
		return nil, s.reopen(d.TakeID)
	}
	return nil, fmt.Errorf("未知事件类型 %q", ev.Op)
}

func (s *State) addStock(loc, sku string, delta int) {
	m, ok := s.Stock[loc]
	if !ok {
		m = map[string]int{}
		s.Stock[loc] = m
	}
	m[sku] += delta
}

// SortedStock 返回排序后的库存快照，供查询输出。
func (s *State) SortedStock(locFilter string) []StockRow {
	var rows []StockRow
	for loc, m := range s.Stock {
		if locFilter != "" && loc != locFilter {
			continue
		}
		for sku, qty := range m {
			rows = append(rows, StockRow{Loc: loc, SKU: sku, Qty: qty})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Loc != rows[j].Loc {
			return rows[i].Loc < rows[j].Loc
		}
		return rows[i].SKU < rows[j].SKU
	})
	return rows
}
