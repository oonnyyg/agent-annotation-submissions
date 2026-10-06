package elfmap

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// HexU64 marshals as a 0x-prefixed hex string in JSON.
type HexU64 uint64

func (h HexU64) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("0x%x", uint64(h)))
}

func (h *HexU64) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	if err != nil {
		return fmt.Errorf("invalid hex address %q", s)
	}
	*h = HexU64(v)
	return nil
}

type ReadQuery struct {
	Vaddr  HexU64 `json:"vaddr"`
	Length uint64 `json:"length"`
}

type Query struct {
	ELFPath  string      `json:"elf_path"`
	LoadBias HexU64      `json:"load_bias"`
	Reads    []ReadQuery `json:"reads"`
}

type SegmentReport struct {
	Index  int    `json:"index"`
	Flags  string `json:"flags"`
	Off    HexU64 `json:"file_offset"`
	Vaddr  HexU64 `json:"vaddr"`
	Filesz HexU64 `json:"file_size"`
	Memsz  HexU64 `json:"mem_size"`
	Align  HexU64 `json:"align"`
}

type PageReport struct {
	Page  HexU64 `json:"page"`
	Flags string `json:"flags"`
}

type ByteSource struct {
	Vaddr  HexU64 `json:"vaddr"`
	Length HexU64 `json:"length"`
	Source string `json:"source"` // "file" or "zero"
}

type ReadResult struct {
	Vaddr   HexU64       `json:"vaddr"`
	Length  uint64       `json:"length"`
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
	Data    string       `json:"data,omitempty"` // hex, only when small
	Regions []ByteSource `json:"regions,omitempty"`
}

type Report struct {
	ELFPath     string          `json:"elf_path"`
	Type        string          `json:"type"`
	Machine     string          `json:"machine"`
	LoadBias    HexU64          `json:"load_bias"`
	Entry       HexU64          `json:"entry"`
	EntryStatus string          `json:"entry_status"`
	Segments    []SegmentReport `json:"segments"`
	Pages       []PageReport    `json:"pages"`
	WXPages     []HexU64        `json:"wx_pages"`
	ReadResults []ReadResult    `json:"read_results"`
}
