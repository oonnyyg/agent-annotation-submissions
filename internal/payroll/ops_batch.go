package payroll

import (
	"errors"
	"fmt"
)

// registerBatch 登记生产批次。先完成全部校验再修改状态，
// 失败不会留下部分成员收入或占用编号。
func (s *Service) registerBatch(req Request) ([]EmpAmount, int, error) {
	if req.BatchID == "" {
		return nil, 0, errors.New("batch_id required")
	}
	if !validMonth(req.Month) {
		return nil, 0, errors.New("month must be YYYY-MM")
	}
	fp := fingerprint(req.BatchID, req.SchemeID, req.Month, req.Quantity, req.Members)
	if existing, ok := s.state.Batches[req.BatchID]; ok {
		if existing.Fingerprint == fp {
			// 相同内容重提：返回原结果
			shares := map[string]int{}
			for _, e := range s.state.Entries {
				if e.Kind == "add" && e.BatchID == req.BatchID {
					shares[e.EmpID] = e.Amount
				}
			}
			return sortedShares(shares), existing.Total, nil
		}
		return nil, 0, fmt.Errorf("batch %s already registered with different content", req.BatchID)
	}
	if s.state.ClosedMonths[req.Month] {
		return nil, 0, fmt.Errorf("month %s is closed", req.Month)
	}
	sv := s.latestScheme(req.SchemeID)
	if sv == nil {
		return nil, 0, fmt.Errorf("scheme %s not registered", req.SchemeID)
	}
	total, err := Price(sv.Tiers, req.Quantity)
	if err != nil {
		return nil, 0, err
	}
	shares, err := Distribute(total, req.Members)
	if err != nil {
		return nil, 0, err
	}
	b := &Batch{
		BatchID:       req.BatchID,
		SchemeID:      req.SchemeID,
		SchemeVersion: sv.Version,
		Month:         req.Month,
		Quantity:      req.Quantity,
		Members:       req.Members,
		Total:         total,
		Fingerprint:   fp,
	}
	s.state.Batches[req.BatchID] = b
	for _, m := range req.Members {
		s.state.Entries = append(s.state.Entries, Entry{
			Kind: "add", BatchID: req.BatchID, EmpID: m.EmpID,
			Amount: shares[m.EmpID], Month: req.Month,
			SchemeID: sv.SchemeID, SchemeVersion: sv.Version,
			Quantity: req.Quantity,
		})
	}
	if err := s.save(); err != nil {
		return nil, 0, err
	}
	return sortedShares(shares), total, nil
}

// voidBatch 整批撤销：在指定的未封账月份逐人冲销原分配
// 金额，不重算权重或价格，且不能重复撤销。
func (s *Service) voidBatch(req Request) ([]EmpAmount, error) {
	b, ok := s.state.Batches[req.BatchID]
	if !ok {
		return nil, fmt.Errorf("batch %s not found", req.BatchID)
	}
	if !validMonth(req.Month) {
		return nil, errors.New("month must be YYYY-MM")
	}
	if b.Voided {
		return nil, fmt.Errorf("batch %s already voided", req.BatchID)
	}
	if s.state.ClosedMonths[req.Month] {
		return nil, fmt.Errorf("month %s is closed", req.Month)
	}
	orig := map[string]int{}
	for _, e := range s.state.Entries {
		if e.Kind == "add" && e.BatchID == req.BatchID {
			orig[e.EmpID] = e.Amount
		}
	}
	for emp, amt := range orig {
		s.state.Entries = append(s.state.Entries, Entry{
			Kind: "void", BatchID: req.BatchID, EmpID: emp,
			Amount: -amt, Month: req.Month,
			SchemeID: b.SchemeID, SchemeVersion: b.SchemeVersion,
			Quantity: b.Quantity,
		})
	}
	b.Voided = true
	b.VoidMonth = req.Month
	if err := s.save(); err != nil {
		return nil, err
	}
	neg := map[string]int{}
	for emp, amt := range orig {
		neg[emp] = -amt
	}
	return sortedShares(neg), nil
}

func (s *Service) closeMonth(req Request) error {
	if !validMonth(req.Month) {
		return errors.New("month must be YYYY-MM")
	}
	if s.state.ClosedMonths[req.Month] {
		return fmt.Errorf("month %s already closed", req.Month)
	}
	s.state.ClosedMonths[req.Month] = true
	return s.save()
}

func (s *Service) queryMonth(req Request) (*MonthReport, error) {
	if !validMonth(req.Month) {
		return nil, errors.New("month must be YYYY-MM")
	}
	rep := &MonthReport{
		Month:     req.Month,
		Closed:    s.state.ClosedMonths[req.Month],
		Additions: []Entry{},
		Reversals: []Entry{},
	}
	net := map[string]int{}
	for _, e := range s.state.Entries {
		if e.Month != req.Month {
			continue
		}
		if e.Kind == "add" {
			rep.Additions = append(rep.Additions, e)
		} else {
			rep.Reversals = append(rep.Reversals, e)
		}
		net[e.EmpID] += e.Amount
	}
	rep.Net = sortedShares(net)
	return rep, nil
}
