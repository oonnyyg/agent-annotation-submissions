package natreplay

import (
	"fmt"
	"sort"
)

type endpoint struct {
	ip   uint32
	port uint16
}

type mapping struct {
	internal  endpoint
	public    endpoint
	expiresAt int64
	allowed   map[endpoint]bool
}

// Table 是确定性的 NAT 状态表，不触碰真实网络。
type Table struct {
	cfg       Config
	pool      []endpoint // 已按 (ip, port) 升序排序，含保留端口标记
	reserved  map[endpoint]bool
	byInternal map[endpoint]*mapping
	byPublic   map[endpoint]*mapping
}

func NewTable(cfg Config) (*Table, error) {
	if cfg.InboundMode != "open" && cfg.InboundMode != "restricted" {
		return nil, fmt.Errorf("inbound_mode must be \"open\" or \"restricted\"")
	}
	if cfg.IdleTimeoutMS <= 0 {
		return nil, fmt.Errorf("idle_timeout_ms must be positive")
	}
	t := &Table{
		cfg:        cfg,
		reserved:   map[endpoint]bool{},
		byInternal: map[endpoint]*mapping{},
		byPublic:   map[endpoint]*mapping{},
	}
	for _, p := range cfg.Pool {
		ip, err := parseIPv4(p.IP)
		if err != nil {
			return nil, err
		}
		if p.PortEnd < p.PortStart {
			return nil, fmt.Errorf("pool entry %s has empty port range", p.IP)
		}
		for port := p.PortStart; port <= p.PortEnd; port++ {
			t.pool = append(t.pool, endpoint{ip, port})
			if port == 65535 {
				break
			}
		}
	}
	sort.Slice(t.pool, func(i, j int) bool {
		if t.pool[i].ip != t.pool[j].ip {
			return t.pool[i].ip < t.pool[j].ip
		}
		return t.pool[i].port < t.pool[j].port
	})
	for _, e := range t.pool {
		for _, r := range cfg.ReservedPorts {
			if e.port == r {
				t.reserved[e] = true
			}
		}
	}
	return t, nil
}

// expire 删除截止时间不晚于 now 的映射，并丢弃其允许列表。
func (t *Table) expire(now int64) {
	for _, m := range t.byInternal {
		if m.expiresAt <= now {
			delete(t.byInternal, m.internal)
			delete(t.byPublic, m.public)
		}
	}
}

// allocate 优先保留源端口，再按 (公网地址数值, 端口) 升序选空位。
func (t *Table) allocate(srcPort uint16) (endpoint, bool) {
	for _, e := range t.pool {
		if e.port == srcPort && !t.reserved[e] {
			if _, used := t.byPublic[e]; !used {
				return e, true
			}
		}
	}
	for _, e := range t.pool {
		if t.reserved[e] {
			continue
		}
		if _, used := t.byPublic[e]; !used {
			return e, true
		}
	}
	return endpoint{}, false
}
