package payroll

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Request 是一行 JSONL 操作。op 取值：
// register_scheme / register_batch / void_batch /
// close_month / query_month。
type Request struct {
	Op       string         `json:"op"`
	SchemeID string         `json:"scheme_id,omitempty"`
	Version  int            `json:"version,omitempty"`
	Tiers    []Tier         `json:"tiers,omitempty"`
	BatchID  string         `json:"batch_id,omitempty"`
	Month    string         `json:"month,omitempty"`
	Quantity int            `json:"quantity,omitempty"`
	Members  []MemberWeight `json:"members,omitempty"`
}

// Response 是每个操作对应的一行 JSON 输出。
type Response struct {
	OK       bool        `json:"ok"`
	Op       string      `json:"op"`
	Error    string      `json:"error,omitempty"`
	BatchID  string      `json:"batch_id,omitempty"`
	Total    int         `json:"total,omitempty"`
	Shares   []EmpAmount `json:"shares,omitempty"`
	Report   *MonthReport `json:"report,omitempty"`
}

type EmpAmount struct {
	EmpID  string `json:"emp_id"`
	Amount int    `json:"amount"`
}

// MonthReport 是月度结算查询结果。
type MonthReport struct {
	Month     string       `json:"month"`
	Closed    bool         `json:"closed"`
	Additions []Entry      `json:"additions"`
	Reversals []Entry      `json:"reversals"`
	Net       []EmpAmount  `json:"net"`
}

// Service 持有状态并负责持久化。
type Service struct {
	dir   string
	state *State
}

func Open(dir string) (*Service, error) {
	s := &Service{dir: dir, state: NewState()}
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s.state); err != nil {
		return nil, fmt.Errorf("corrupt state file: %w", err)
	}
	return s, nil
}

func (s *Service) save() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "state.json"))
}

func validMonth(m string) bool {
	if len(m) != 7 || m[4] != '-' {
		return false
	}
	for i, c := range m {
		if i == 4 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func fingerprint(parts ...any) string {
	h := sha256.New()
	b, _ := json.Marshal(parts)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func sortedShares(m map[string]int) []EmpAmount {
	out := make([]EmpAmount, 0, len(m))
	for emp, amt := range m {
		out = append(out, EmpAmount{EmpID: emp, Amount: amt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EmpID < out[j].EmpID })
	return out
}
