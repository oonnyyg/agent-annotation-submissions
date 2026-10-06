package nat

import (
	"fmt"
	"sort"
)

func (t *Table) Process(ev Event) (Result, error) {
	t.expire(ev.Time)
	switch ev.Type {
	case "outbound":
		return t.outbound(ev)
	case "inbound":
		return t.inbound(ev)
	case "query":
		return Result{Time: ev.Time, Type: ev.Type, Action: "query", Active: t.activeMappings()}, nil
	default:
		return Result{}, fmt.Errorf("unknown event type: %q", ev.Type)
	}
}

func (t *Table) outbound(ev Event) (Result, error) {
	srcIP, err := parseIPv4(ev.SrcIP)
	if err != nil {
		return Result{}, err
	}
	dstIP, err := parseIPv4(ev.DstIP)
	if err != nil {
		return Result{}, err
	}
	src := endpoint{srcIP, ev.SrcPort}
	dst := endpoint{dstIP, ev.DstPort}
	res := Result{Time: ev.Time, Type: ev.Type}
	e, ok := t.byInt[src]
	if !ok {
		pub, found := t.allocate(ev.SrcPort)
		if !found {
			res.Action = "dropped"
			res.Reason = "capacity_exhausted"
			t.summary.Drops["capacity_exhausted"]++
			return res, nil
		}
		e = &entry{internal: src, public: pub, allowed: map[endpoint]bool{}}
		t.byInt[src] = e
		t.byPub[pub] = e
		res.Created = true
	}
	e.expireAt = ev.Time + t.timeout
	e.allowed[dst] = true
	res.Action = "forwarded"
	m := mappingOf(e)
	res.Mapping = &m
	t.summary.OutboundForward++
	return res, nil
}

func (t *Table) inbound(ev Event) (Result, error) {
	srcIP, err := parseIPv4(ev.SrcIP)
	if err != nil {
		return Result{}, err
	}
	dstIP, err := parseIPv4(ev.DstIP)
	if err != nil {
		return Result{}, err
	}
	res := Result{Time: ev.Time, Type: ev.Type}
	e, ok := t.byPub[endpoint{dstIP, ev.DstPort}]
	if !ok {
		res.Action = "dropped"
		res.Reason = "no_mapping"
		t.summary.Drops["no_mapping"]++
		return res, nil
	}
	if t.mode == "restricted" && !e.allowed[endpoint{srcIP, ev.SrcPort}] {
		res.Action = "dropped"
		res.Reason = "filtered"
		t.summary.Drops["filtered"]++
		return res, nil
	}
	res.Action = "forwarded"
	m := mappingOf(e)
	res.Mapping = &m
	t.summary.InboundForward++
	return res, nil
}

func mappingOf(e *entry) Mapping {
	return Mapping{
		InternalIP:   ipString(e.internal.ip),
		InternalPort: e.internal.port,
		PublicIP:     ipString(e.public.ip),
		PublicPort:   e.public.port,
		ExpiresAt:    e.expireAt,
	}
}

func (t *Table) activeMappings() []Mapping {
	out := make([]Mapping, 0, len(t.byPub))
	for _, e := range t.byPub {
		out = append(out, mappingOf(e))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PublicIP != out[j].PublicIP {
			return out[i].PublicIP < out[j].PublicIP
		}
		return out[i].PublicPort < out[j].PublicPort
	})
	return out
}
