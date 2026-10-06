package nat

import (
	"fmt"
	"sort"
)

type endpoint struct {
	ip   uint32
	port int
}

type entry struct {
	internal endpoint
	public   endpoint
	expireAt int64
	allowed  map[endpoint]bool
}

type Table struct {
	addrs    []uint32
	reserved map[int]bool
	timeout  int64
	mode     string
	byInt    map[endpoint]*entry
	byPub    map[endpoint]*entry
	summary  Summary
}

func NewTable(cfg Config) (*Table, error) {
	if cfg.Mode != "open" && cfg.Mode != "restricted" {
		return nil, fmt.Errorf("mode must be \"open\" or \"restricted\"")
	}
	if cfg.IdleTimeoutMs <= 0 {
		return nil, fmt.Errorf("idle_timeout_ms must be positive")
	}
	reserved, err := parseReserved(cfg.ReservedPorts)
	if err != nil {
		return nil, err
	}
	t := &Table{
		reserved: reserved,
		timeout:  cfg.IdleTimeoutMs,
		mode:     cfg.Mode,
		byInt:    map[endpoint]*entry{},
		byPub:    map[endpoint]*entry{},
		summary:  Summary{Type: "summary", Drops: map[string]int{}},
	}
	for _, s := range cfg.PublicAddresses {
		v, err := parseIPv4(s)
		if err != nil {
			return nil, err
		}
		t.addrs = append(t.addrs, v)
	}
	if len(t.addrs) == 0 {
		return nil, fmt.Errorf("public_addresses must not be empty")
	}
	sort.Slice(t.addrs, func(i, j int) bool { return t.addrs[i] < t.addrs[j] })
	return t, nil
}

func (t *Table) Summary() Summary { return t.summary }

func (t *Table) expire(now int64) {
	for pub, e := range t.byPub {
		if e.expireAt <= now {
			delete(t.byPub, pub)
			delete(t.byInt, e.internal)
		}
	}
}

func (t *Table) allocate(srcPort int) (endpoint, bool) {
	for _, addr := range t.addrs {
		cand := endpoint{addr, srcPort}
		if !t.reserved[srcPort] {
			if _, used := t.byPub[cand]; !used {
				return cand, true
			}
		}
	}
	for _, addr := range t.addrs {
		for p := 0; p <= 65535; p++ {
			if t.reserved[p] || p == srcPort {
				continue
			}
			cand := endpoint{addr, p}
			if _, used := t.byPub[cand]; !used {
				return cand, true
			}
		}
	}
	return endpoint{}, false
}
