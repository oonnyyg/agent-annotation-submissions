package main

import (
	"fmt"
	"sort"
)

// Request 是一行 JSONL 操作。
type Request struct {
	Op       string           `json:"op"`
	SchemeID string           `json:"scheme_id,omitempty"`
	Tiers    []Tier           `json:"tiers,omitempty"`
	BatchID  string           `json:"batch_id,omitempty"`
	Month    string           `json:"month,omitempty"`
	Qty      int64            `json:"qty,omitempty"`
	Members  map[string]int64 `json:"members,omitempty"`
}

// Response 是每个操作对应的一行 JSON 输出。
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	SchemeID      string `json:"scheme_id,omitempty"`
	SchemeVersion int    `json:"scheme_version,omitempty"`

	BatchID    string           `json:"batch_id,omitempty"`
	Month      string           `json:"month,omitempty"`
	TotalCents int64            `json:"total_cents,omitempty"`
	Shares     map[string]int64 `json:"shares,omitempty"`
	Voided     bool             `json:"voided,omitempty"`
	VoidMonth  string           `json:"void_month,omitempty"`
	Closed     bool             `json:"closed,omitempty"`
	Duplicate  bool             `json:"duplicate,omitempty"`

	Report *MonthReport `json:"report,omitempty"`
}

func fail(format string, args ...any) Response {
	return Response{OK: false, Error: fmt.Sprintf(format, args...)}
}

// dispatch 执行单个操作；返回响应与是否修改了状态。
func dispatch(st *State, req Request) (Response, bool) {
	switch req.Op {
	case "register_scheme":
		return opRegisterScheme(st, req)
	case "register_batch":
		return opRegisterBatch(st, req)
	case "void_batch":
		return opVoidBatch(st, req)
	case "close_month":
		return opCloseMonth(st, req)
	case "query_month":
		return opQueryMonth(st, req)
	default:
		return fail("未知操作: %q", req.Op), false
	}
}

// opRegisterScheme 登记新方案或为已有方案追加新版本。
// 已登记批次保留当时的版本，不受更新影响。
func opRegisterScheme(st *State, req Request) (Response, bool) {
	if req.SchemeID == "" {
		return fail("scheme_id 不能为空"), false
	}
	if err := validateTiers(req.Tiers); err != nil {
		return fail("阶梯无效: %v", err), false
	}
	tiers := make([]Tier, len(req.Tiers))
	copy(tiers, req.Tiers)
	sc, ok := st.Schemes[req.SchemeID]
	if !ok {
		sc = &Scheme{ID: req.SchemeID}
		st.Schemes[req.SchemeID] = sc
	}
	ver := len(sc.Versions) + 1
	sc.Versions = append(sc.Versions, SchemeVersion{Version: ver, Tiers: tiers})
	return Response{OK: true, SchemeID: req.SchemeID, SchemeVersion: ver}, true
}

func (s *Scheme) version(v int) (*SchemeVersion, error) {
	if v < 1 || v > len(s.Versions) {
		return nil, fmt.Errorf("方案 %s 不存在版本 %d", s.ID, v)
	}
	return &s.Versions[v-1], nil
}

// batchFingerprint 生成批次请求内容的规范化指纹，用于幂等判定。
func batchFingerprint(req Request) string {
	return fmt.Sprintf("scheme=%s|month=%s|qty=%d|members=%s",
		req.SchemeID, req.Month, req.Qty, canonicalMembers(req.Members))
}

func batchResponse(b *Batch, dup bool) Response {
	shares := make(map[string]int64, len(b.Shares))
	for k, v := range b.Shares {
		shares[k] = v
	}
	return Response{
		OK:            true,
		BatchID:       b.ID,
		SchemeID:      b.SchemeID,
		SchemeVersion: b.SchemeVersion,
		Month:         b.Month,
		TotalCents:    b.TotalCents,
		Shares:        shares,
		Duplicate:     dup,
	}
}

// opRegisterBatch 登记生产批次并按当前方案版本计价、按权重分配。
// 编号唯一：内容相同的重提返回原结果，内容不同则拒绝；
// 任何校验失败都不会写入状态或占用编号。
func opRegisterBatch(st *State, req Request) (Response, bool) {
	if req.BatchID == "" {
		return fail("batch_id 不能为空"), false
	}
	fp := batchFingerprint(req)
	if existing, ok := st.Batches[req.BatchID]; ok {
		if st.Requests[req.BatchID] == fp {
			return batchResponse(existing, true), false
		}
		return fail("批次编号 %s 已存在且内容不同", req.BatchID), false
	}
	if req.Month == "" {
		return fail("month 不能为空"), false
	}
	if st.monthClosed(req.Month) {
		return fail("月份 %s 已封账，不能新增记录", req.Month), false
	}
	if req.Qty <= 0 {
		return fail("qty 必须为正整数"), false
	}
	sc, ok := st.Schemes[req.SchemeID]
	if !ok {
		return fail("方案 %s 不存在", req.SchemeID), false
	}
	ver, err := sc.version(len(sc.Versions))
	if err != nil {
		return fail("%v", err), false
	}
	total := priceQty(ver.Tiers, req.Qty)
	shares, err := splitShares(req.Members, total)
	if err != nil {
		return fail("分配失败: %v", err), false
	}
	members := make(map[string]int64, len(req.Members))
	for k, v := range req.Members {
		members[k] = v
	}
	b := &Batch{
		ID:            req.BatchID,
		SchemeID:      req.SchemeID,
		SchemeVersion: ver.Version,
		Month:         req.Month,
		Qty:           req.Qty,
		Members:       members,
		TotalCents:    total,
		Shares:        shares,
	}
	st.Batches[b.ID] = b
	st.Requests[b.ID] = fp
	return batchResponse(b, false), true
}

// opVoidBatch 整批撤销：在指定的未封账月份逐人冲销原分配金额，
// 直接取登记时的份额快照取负，不按当前权重或价格重算，且不可重复撤销。
func opVoidBatch(st *State, req Request) (Response, bool) {
	if req.BatchID == "" {
		return fail("batch_id 不能为空"), false
	}
	b, ok := st.Batches[req.BatchID]
	if !ok {
		return fail("批次 %s 不存在", req.BatchID), false
	}
	if b.Voided {
		return fail("批次 %s 已撤销，不能重复撤销", req.BatchID), false
	}
	if req.Month == "" {
		return fail("month 不能为空"), false
	}
	if st.monthClosed(req.Month) {
		return fail("月份 %s 已封账，不能记入冲销", req.Month), false
	}
	b.Voided = true
	b.VoidMonth = req.Month
	shares := make(map[string]int64, len(b.Shares))
	for k, v := range b.Shares {
		shares[k] = -v
	}
	return Response{
		OK:            true,
		BatchID:       b.ID,
		SchemeID:      b.SchemeID,
		SchemeVersion: b.SchemeVersion,
		Month:         req.Month,
		TotalCents:    -b.TotalCents,
		Shares:        shares,
		Voided:        true,
		VoidMonth:     req.Month,
	}, true
}

// opCloseMonth 封账：之后该月不能新增批次或冲销记录。
func opCloseMonth(st *State, req Request) (Response, bool) {
	if req.Month == "" {
		return fail("month 不能为空"), false
	}
	m, ok := st.Months[req.Month]
	if !ok {
		m = &MonthBook{}
		st.Months[req.Month] = m
	}
	if m.Closed {
		return fail("月份 %s 已封账", req.Month), false
	}
	m.Closed = true
	return Response{OK: true, Month: req.Month, Closed: true}, true
}

// MemberMonth 是某员工当月的汇总：新增、冲销与净额（分）。
type MemberMonth struct {
	Added    int64 `json:"added"`
	Reversed int64 `json:"reversed"`
	Net      int64 `json:"net"`
}

// MonthEntry 是一笔可追查的流水：指向原批次与登记时的方案版本。
type MonthEntry struct {
	Kind          string `json:"kind"` // "add" 或 "reversal"
	BatchID       string `json:"batch_id"`
	SchemeID      string `json:"scheme_id"`
	SchemeVersion int    `json:"scheme_version"`
	BatchMonth    string `json:"batch_month"`
	Qty           int64  `json:"qty"`
	TotalCents    int64  `json:"total_cents"`
}

// MonthReport 是月度结算查询结果。
type MonthReport struct {
	Month    string                  `json:"month"`
	Closed   bool                    `json:"closed"`
	Members  map[string]*MemberMonth `json:"members"`
	Added    int64                   `json:"added"`
	Reversed int64                   `json:"reversed"`
	Net      int64                   `json:"net"`
	Entries  []MonthEntry            `json:"entries"`
}

// opQueryMonth 查询某月：分别展示当期新增与冲销及净额，并可追查原批次和方案版本。
func opQueryMonth(st *State, req Request) (Response, bool) {
	if req.Month == "" {
		return fail("month 不能为空"), false
	}
	rep := &MonthReport{
		Month:   req.Month,
		Closed:  st.monthClosed(req.Month),
		Members: map[string]*MemberMonth{},
		Entries: []MonthEntry{},
	}
	member := func(id string) *MemberMonth {
		m, ok := rep.Members[id]
		if !ok {
			m = &MemberMonth{}
			rep.Members[id] = m
		}
		return m
	}
	ids := make([]string, 0, len(st.Batches))
	for id := range st.Batches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		b := st.Batches[id]
		if b.Month == req.Month {
			for emp, amt := range b.Shares {
				m := member(emp)
				m.Added += amt
				m.Net += amt
			}
			rep.Added += b.TotalCents
			rep.Net += b.TotalCents
			rep.Entries = append(rep.Entries, MonthEntry{
				Kind: "add", BatchID: b.ID, SchemeID: b.SchemeID,
				SchemeVersion: b.SchemeVersion, BatchMonth: b.Month,
				Qty: b.Qty, TotalCents: b.TotalCents,
			})
		}
		if b.Voided && b.VoidMonth == req.Month {
			for emp, amt := range b.Shares {
				m := member(emp)
				m.Reversed -= amt
				m.Net -= amt
			}
			rep.Reversed -= b.TotalCents
			rep.Net -= b.TotalCents
			rep.Entries = append(rep.Entries, MonthEntry{
				Kind: "reversal", BatchID: b.ID, SchemeID: b.SchemeID,
				SchemeVersion: b.SchemeVersion, BatchMonth: b.Month,
				Qty: b.Qty, TotalCents: -b.TotalCents,
			})
		}
	}
	return Response{OK: true, Month: req.Month, Report: rep}, false
}
