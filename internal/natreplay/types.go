package natreplay

import (
	"encoding/json"
	"fmt"
	"net"
)

// Config 描述地址池、保留端口、空闲超时与回包过滤模式。
type Config struct {
	Pool          []PoolEntry `json:"pool"`
	ReservedPorts []uint16    `json:"reserved_ports"`
	IdleTimeoutMS int64       `json:"idle_timeout_ms"`
	InboundMode   string      `json:"inbound_mode"` // "open" 或 "restricted"
}

type PoolEntry struct {
	IP        string `json:"ip"`
	PortStart uint16 `json:"port_start"`
	PortEnd   uint16 `json:"port_end"`
}

// Event 是一条 JSONL 输入事件。Type 为 outbound / inbound / query。
type Event struct {
	Type    string `json:"type"`
	Time    int64  `json:"time"`
	SrcIP   string `json:"src_ip,omitempty"`
	SrcPort uint16 `json:"src_port,omitempty"`
	DstIP   string `json:"dst_ip,omitempty"`
	DstPort uint16 `json:"dst_port,omitempty"`
}

// Result 是每个事件处理后输出的一行 JSON。
type Result struct {
	Seq       int    `json:"seq"`
	Time      int64  `json:"time"`
	Type      string `json:"type"`
	Action    string `json:"action"` // forward / drop / query
	Reason    string `json:"reason,omitempty"`
	Mapping   string `json:"mapping,omitempty"`
	Public    string `json:"public,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// Summary 在全部事件处理后输出。
type Summary struct {
	ForwardedOut int           `json:"forwarded_out"`
	ForwardedIn  int           `json:"forwarded_in"`
	Drops        map[string]int `json:"drops"`
	Active       []MappingView `json:"active_mappings"`
}

type MappingView struct {
	Internal  string   `json:"internal"`
	Public    string   `json:"public"`
	ExpiresAt int64    `json:"expires_at"`
	Allowed   []string `json:"allowed,omitempty"`
}

func parseIPv4(s string) (uint32, error) {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid IPv4 address %q", s)
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3]), nil
}

func ipv4String(v uint32) string {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String()
}

func addrStr(ip uint32, port uint16) string {
	return fmt.Sprintf("%s:%d", ipv4String(ip), port)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
