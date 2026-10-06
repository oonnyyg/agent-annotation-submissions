package nat

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Config struct {
	PublicAddresses []string `json:"public_addresses"`
	ReservedPorts   []string `json:"reserved_ports"`
	IdleTimeoutMs   int64    `json:"idle_timeout_ms"`
	Mode            string   `json:"mode"`
}

type Event struct {
	Time    int64  `json:"time"`
	Type    string `json:"type"`
	SrcIP   string `json:"src_ip,omitempty"`
	SrcPort int    `json:"src_port,omitempty"`
	DstIP   string `json:"dst_ip,omitempty"`
	DstPort int    `json:"dst_port,omitempty"`
}

type Mapping struct {
	InternalIP   string `json:"internal_ip"`
	InternalPort int    `json:"internal_port"`
	PublicIP     string `json:"public_ip"`
	PublicPort   int    `json:"public_port"`
	ExpiresAt    int64  `json:"expires_at"`
}

type Result struct {
	Time    int64     `json:"time"`
	Type    string    `json:"type"`
	Action  string    `json:"action"`
	Reason  string    `json:"reason,omitempty"`
	Mapping *Mapping  `json:"mapping,omitempty"`
	Created bool      `json:"created,omitempty"`
	Active  []Mapping `json:"active,omitempty"`
}

type Summary struct {
	Type            string         `json:"type"`
	OutboundForward int            `json:"outbound_forwarded"`
	InboundForward  int            `json:"inbound_forwarded"`
	Drops           map[string]int `json:"drops"`
}

func parseIPv4(s string) (uint32, error) {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid IPv4 address: %q", s)
	}
	return binary.BigEndian.Uint32(ip), nil
}

func ipString(v uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return net.IP(b[:]).String()
}

func parseReserved(specs []string) (map[int]bool, error) {
	reserved := map[int]bool{}
	for _, spec := range specs {
		lo, hi, err := parsePortRange(spec)
		if err != nil {
			return nil, err
		}
		for p := lo; p <= hi; p++ {
			reserved[p] = true
		}
	}
	return reserved, nil
}

func parsePortRange(spec string) (int, int, error) {
	parts := strings.SplitN(spec, "-", 2)
	lo, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || lo < 0 || lo > 65535 {
		return 0, 0, fmt.Errorf("invalid reserved port: %q", spec)
	}
	hi := lo
	if len(parts) == 2 {
		hi, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || hi < lo || hi > 65535 {
			return 0, 0, fmt.Errorf("invalid reserved port range: %q", spec)
		}
	}
	return lo, hi, nil
}
