package elfmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadConfig reads and parses the JSON query configuration. Addresses may be
// JSON numbers or strings such as "0x401000".
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(data)
}

type rawConfig struct {
	LoadBias json.RawMessage `json:"load_bias"`
	Reads    []struct {
		Name   string          `json:"name"`
		Addr   json.RawMessage `json:"addr"`
		Length int             `json:"length"`
	} `json:"reads"`
}

// ParseConfig parses config JSON with hex/decimal address flexibility.
func ParseConfig(data []byte) (Config, error) {
	var raw rawConfig
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("invalid config JSON: %w", err)
	}
	cfg := Config{Reads: []ReadQuery{}}
	if len(raw.LoadBias) > 0 {
		v, err := parseUintField(raw.LoadBias)
		if err != nil {
			return Config{}, fmt.Errorf("load_bias: %w", err)
		}
		cfg.LoadBias = v
	}
	for i, q := range raw.Reads {
		addr, err := parseUintField(q.Addr)
		if err != nil {
			return Config{}, fmt.Errorf("reads[%d] (%q): %w", i, q.Name, err)
		}
		if q.Length < 0 {
			return Config{}, fmt.Errorf("reads[%d] (%q): negative length", i, q.Name)
		}
		cfg.Reads = append(cfg.Reads, ReadQuery{
			Name:   q.Name,
			Addr:   addr,
			Length: q.Length,
		})
	}
	return cfg, nil
}

func parseUintField(raw json.RawMessage) (uint64, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0, nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return 0, err
		}
		s = strings.TrimSpace(str)
	}
	return strconv.ParseUint(s, 0, 64)
}
