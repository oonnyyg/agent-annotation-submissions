package analyzer

import "fmt"

// MatchRule 判断规则是否命中给定输入。输入值类型须与字段类型匹配。
func MatchRule(s *Spec, rule *Rule, input map[string]any) (bool, error) {
	for _, f := range s.Fields {
		cond, ok := rule.When[f.Name]
		if !ok {
			continue // 省略字段表示不限
		}
		v, present := input[f.Name]
		if !present {
			return false, fmt.Errorf("输入缺少字段 %q", f.Name)
		}
		switch f.Type {
		case FieldEnum:
			sv, ok := v.(string)
			if !ok {
				return false, fmt.Errorf("字段 %q 期望字符串，得到 %v", f.Name, v)
			}
			if !f.HasValue(sv) {
				return false, fmt.Errorf("字段 %q 的值 %q 不在域内", f.Name, sv)
			}
			found := false
			for _, ev := range cond.Enum {
				if ev == sv {
					found = true
					break
				}
			}
			if !found {
				return false, nil
			}
		case FieldInt:
			iv, ok := toInt(v)
			if !ok {
				return false, fmt.Errorf("字段 %q 期望整数，得到 %v", f.Name, v)
			}
			if iv < f.Min || iv > f.Max {
				return false, fmt.Errorf("字段 %q 的值 %d 超出域 [%d, %d]", f.Name, iv, f.Min, f.Max)
			}
			lo, hi, err := NormalizeRange(&f, cond.Range)
			if err != nil {
				return false, err
			}
			if iv < lo || iv > hi {
				return false, nil
			}
		}
	}
	return true, nil
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
}

// MatchedRules 返回命中输入的全部规则下标（按声明顺序）。
func MatchedRules(s *Spec, input map[string]any) ([]int, error) {
	var out []int
	for i := range s.Rules {
		ok, err := MatchRule(s, &s.Rules[i], input)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, i)
		}
	}
	return out, nil
}

// MatchCell 返回命中单元代表输入的全部规则下标（单元内一致）。
func MatchCell(s *Spec, c Cell) []int {
	input := s.Representative(c)
	matched, err := MatchedRules(s, input)
	if err != nil {
		panic(err) // 代表输入由域构造，不会出错
	}
	return matched
}
