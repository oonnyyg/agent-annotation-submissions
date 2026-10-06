package natreplay

import (
	"fmt"
	"sort"
)

// Replayer 按事件时间非递减顺序驱动 NAT 表。
type Replayer struct {
	table *Table
	seq   int
	sum   Summary
}

func NewReplayer(t *Table) *Replayer {
	return &Replayer{table: t, sum: Summary{Drops: map[string]int{}}}
}

// Handle 处理一条事件：先清理到期映射，再按类型分派。
func (r *Replayer) Handle(ev Event) (Result, error) {
	r.seq++
	r.table.expire(ev.Time)
	res := Result{Seq: r.seq, Time: ev.Time, Type: ev.Type}
	var err error
	switch ev.Type {
	case "outbound":
		res, err = r.outbound(ev, res)
	case "inbound":
		res, err = r.inbound(ev, res)
	case "query":
		res.Action = "query"
	default:
		return res, fmt.Errorf("event %d: unknown type %q", r.seq, ev.Type)
	}
	return res, err
}

func (r *Replayer) outbound(ev Event, res Result) (Result, error) {
	srcIP, err := parseIPv4(ev.SrcIP)
	if err != nil {
		return res, fmt.Errorf("event %d: %w", r.seq, err)
	}
	dstIP, err := parseIPv4(ev.DstIP)
	if err != nil {
		return res, fmt.Errorf("event %d: %w", r.seq, err)
	}
	internal := endpoint{srcIP, ev.SrcPort}
	m, ok := r.table.byInternal[internal]
	if !ok {
		pub, ok := r.table.allocate(ev.SrcPort)
		if !ok {
			res.Action = "drop"
			res.Reason = "no_capacity"
			r.sum.Drops["no_capacity"]++
			return res, nil
		}
		m = &mapping{internal: internal, public: pub, allowed: map[endpoint]bool{}}
		r.table.byInternal[internal] = m
		r.table.byPublic[pub] = m
	}
	m.expiresAt = ev.Time + r.table.cfg.IdleTimeoutMS
	m.allowed[endpoint{dstIP, ev.DstPort}] = true
	res.Action = "forward"
	res.Mapping = addrStr(m.internal.ip, m.internal.port)
	res.Public = addrStr(m.public.ip, m.public.port)
	res.ExpiresAt = m.expiresAt
	r.sum.ForwardedOut++
	return res, nil
}

func (r *Replayer) inbound(ev Event, res Result) (Result, error) {
	srcIP, err := parseIPv4(ev.SrcIP)
	if err != nil {
		return res, fmt.Errorf("event %d: %w", r.seq, err)
	}
	dstIP, err := parseIPv4(ev.DstIP)
	if err != nil {
		return res, fmt.Errorf("event %d: %w", r.seq, err)
	}
	m, ok := r.table.byPublic[endpoint{dstIP, ev.DstPort}]
	if !ok {
		res.Action = "drop"
		res.Reason = "unmapped"
		r.sum.Drops["unmapped"]++
		return res, nil
	}
	if r.table.cfg.InboundMode == "restricted" && !m.allowed[endpoint{srcIP, ev.SrcPort}] {
		res.Action = "drop"
		res.Reason = "filtered"
		res.Mapping = addrStr(m.internal.ip, m.internal.port)
		r.sum.Drops["filtered"]++
		return res, nil
	}
	res.Action = "forward"
	res.Mapping = addrStr(m.internal.ip, m.internal.port)
	res.Public = addrStr(m.public.ip, m.public.port)
	res.ExpiresAt = m.expiresAt
	r.sum.ForwardedIn++
	return res, nil
}

// Summary 返回当前汇总；Active 按公网地址端口升序，允许列表排序输出。
func (r *Replayer) Summary() Summary {
	out := r.sum
	for _, m := range r.table.byPublic {
		view := MappingView{
			Internal:  addrStr(m.internal.ip, m.internal.port),
			Public:    addrStr(m.public.ip, m.public.port),
			ExpiresAt: m.expiresAt,
		}
		for a := range m.allowed {
			view.Allowed = append(view.Allowed, addrStr(a.ip, a.port))
		}
		sort.Strings(view.Allowed)
		out.Active = append(out.Active, view)
	}
	sort.Slice(out.Active, func(i, j int) bool { return out.Active[i].Public < out.Active[j].Public })
	return out
}
