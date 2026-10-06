package ruleaudit

import (
	"encoding/json"
	"fmt"
)

// EvalResult is the outcome of evaluating one concrete input.
type EvalResult struct {
	Matched []int // all matching rule indices, in rule order
	Rule    int   // winning rule, or -1
	Output  json.RawMessage
	OK      bool // a definite outcome was produced
	Reason  string
}

// Evaluate matches a concrete input against the rules using the given
// strategy. It shares the matcher with the static analysis, so results
// are consistent by construction.
func Evaluate(spec *Spec, strategy Strategy, input Assignment) (*EvalResult, error) {
	// Validate the input against declared domains.
	for i := range spec.Fields {
		f := &spec.Fields[i]
		v, ok := input[f.Name]
		if !ok {
			return nil, fmt.Errorf("input: missing field %q", f.Name)
		}
		switch f.Type {
		case KindEnum:
			s, ok := v.(string)
			if !ok || !f.HasValue(s) {
				return nil, fmt.Errorf("input: field %q: value %v not in enum domain", f.Name, v)
			}
		case KindInt:
			n, ok := toInt(v)
			if !ok || n < *f.Min || n > *f.Max {
				return nil, fmt.Errorf("input: field %q: value %v outside domain [%d, %d]", f.Name, v, *f.Min, *f.Max)
			}
		}
	}
	for name := range input {
		if spec.FieldByName(name) == nil {
			return nil, fmt.Errorf("input: unknown field %q", name)
		}
	}

	res := &EvalResult{Rule: -1}
	for ri := range spec.Rules {
		if ruleMatchesInput(spec, ri, input) {
			res.Matched = append(res.Matched, ri)
		}
	}
	switch strategy {
	case StrategyUnique:
		if len(res.Matched) == 1 {
			res.Rule = res.Matched[0]
		} else if len(res.Matched) == 0 {
			res.Reason = "no rule matched"
		} else {
			res.Reason = fmt.Sprintf("conflict: %d rules matched", len(res.Matched))
		}
	case StrategyFirst:
		if len(res.Matched) > 0 {
			res.Rule = res.Matched[0]
		} else {
			res.Reason = "no rule matched"
		}
	}
	if res.Rule >= 0 {
		res.OK = true
		res.Output = spec.Rules[res.Rule].Output
	}
	return res, nil
}

func ruleMatchesInput(spec *Spec, ri int, input Assignment) bool {
	rule := &spec.Rules[ri]
	for i := range spec.Fields {
		f := &spec.Fields[i]
		if _, ok := rule.When[f.Name]; !ok {
			continue
		}
		if f.Type == KindEnum {
			if !enumMatches(rule, f.Name, input[f.Name].(string)) {
				return false
			}
		} else {
			n, _ := toInt(input[f.Name])
			if !intMatches(rule, f, n) {
				return false
			}
		}
	}
	return true
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
}
