package spf

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Snapshot struct {
	Records map[string]map[string][][]string `json:"records"`
	Errors  map[string]string                `json:"errors"`
}

func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	if s.Records == nil {
		s.Records = map[string]map[string][][]string{}
	}
	if s.Errors == nil {
		s.Errors = map[string]string{}
	}
	norm := map[string]map[string][][]string{}
	for domain, byType := range s.Records {
		norm[strings.ToLower(domain)] = byType
	}
	s.Records = norm
	return &s, nil
}

type lookupError struct{ msg string }

func (e *lookupError) Error() string { return e.msg }

func IsTemp(err error) bool {
	_, ok := err.(*lookupError)
	return ok
}

func (s *Snapshot) Lookup(domain, rrtype string) ([][]string, error) {
	d := strings.ToLower(domain)
	if code, ok := s.Errors[d]; ok {
		return nil, &lookupError{msg: fmt.Sprintf("temporary DNS failure for %s: %s", d, code)}
	}
	byType, ok := s.Records[d]
	if !ok {
		return nil, nil
	}
	return byType[rrtype], nil
}
