package analyzer

import (
	"strings"
	"testing"
)

func intField(name string, min, max int) Field {
	return Field{Name: name, Type: FieldInt, Min: min, Max: max}
}

func ptr(v int) *int { return &v }

func rule(id string, when map[string]Condition) Rule {
	return Rule{ID: id, When: when}
}

// 联合遮蔽：r3 被 r1 与 r2 联合完全遮蔽，单独任何一条都遮不住。
func TestJointShadowing(t *testing.T) {
	spec := &Spec{
		Fields:   []Field{intField("x", 0, 10)},
		Strategy: StrategyFirst,
		Rules: []Rule{
			rule("r1", map[string]Condition{"x": {Range: &IntRange{Min: ptr(0), Max: ptr(5)}}}),
			rule("r2", map[string]Condition{"x": {Range: &IntRange{Min: ptr(4), Max: ptr(8)}}}),
			rule("r3", map[string]Condition{"x": {Range: &IntRange{Min: ptr(2), Max: ptr(6)}}}),
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	rep := Analyze(spec)
	if !rep.Complete {
		t.Fatal("应完成分析")
	}
	if len(rep.Shadowed) != 1 || rep.Shadowed[0].RuleID != "r3" {
		t.Fatalf("r3 应被遮蔽, got %+v", rep.Shadowed)
	}
	got := strings.Join(rep.Shadowed[0].Coverers, ",")
	if got != "r1,r2" {
		t.Fatalf("遮蔽者应为 r1,r2, got %s", got)
	}
	if len(rep.Witnesses) != 2 {
		t.Fatalf("r1、r2 应有见证, got %+v", rep.Witnesses)
	}
	// 见证取最小输入
	if rep.Witnesses[0].RuleID != "r1" || rep.Witnesses[0].Input["x"] != 0 {
		t.Fatalf("r1 见证应为 x=0, got %+v", rep.Witnesses[0])
	}
	if rep.Witnesses[1].RuleID != "r2" || rep.Witnesses[1].Input["x"] != 6 {
		t.Fatalf("r2 见证应为 x=6, got %+v", rep.Witnesses[1])
	}
	if len(rep.Uncovered) != 2 { // [9,10] 与无——实际只有 [9,10]
		if len(rep.Uncovered) != 1 {
			t.Fatalf("未覆盖区域数量不符: %+v", rep.Uncovered)
		}
	}
}

// 开闭端点：exclusive 端点应被排除。
func TestOpenClosedEndpoints(t *testing.T) {
	spec := &Spec{
		Fields:   []Field{intField("x", 0, 10)},
		Strategy: StrategyUnique,
		Rules: []Rule{
			// (2, 5] => 3..5
			rule("r1", map[string]Condition{"x": {Range: &IntRange{Min: ptr(2), MinExclusive: true, Max: ptr(5)}}}),
			// [6, 9) => 6..8
			rule("r2", map[string]Condition{"x": {Range: &IntRange{Min: ptr(6), Max: ptr(9), MaxExclusive: true}}}),
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	rep := Analyze(spec)
	if len(rep.Overlaps) != 0 {
		t.Fatalf("不应有重叠: %+v", rep.Overlaps)
	}
	// 未覆盖单元应包含 x=2（在 [0,2] 内）与 x=9（在 [9,10] 内）
	descs := []string{}
	for _, u := range rep.Uncovered {
		descs = append(descs, u.Desc)
	}
	joined := strings.Join(descs, ";")
	if !strings.Contains(joined, "x∈[0,2]") || !strings.Contains(joined, "x∈[9,10]") {
		t.Fatalf("未覆盖单元不符: %s", joined)
	}
	// 求值一致性
	m, _ := MatchedRules(spec, map[string]any{"x": 2})
	if len(m) != 0 {
		t.Fatal("x=2 不应命中 r1（开端点）")
	}
	m, _ = MatchedRules(spec, map[string]any{"x": 3})
	if len(m) != 1 || m[0] != 0 {
		t.Fatal("x=3 应命中 r1")
	}
	m, _ = MatchedRules(spec, map[string]any{"x": 9})
	if len(m) != 0 {
		t.Fatal("x=9 不应命中 r2（开端点）")
	}
}

// 唯一命中策略：输出相同也算冲突。
func TestUniqueConflictSameOutput(t *testing.T) {
	spec := &Spec{
		Fields:   []Field{intField("x", 0, 10)},
		Strategy: StrategyUnique,
		Rules: []Rule{
			{ID: "r1", Output: "A", When: map[string]Condition{"x": {Range: &IntRange{Min: ptr(0), Max: ptr(5)}}}},
			{ID: "r2", Output: "A", When: map[string]Condition{"x": {Range: &IntRange{Min: ptr(3), Max: ptr(10)}}}},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	rep := Analyze(spec)
	if len(rep.Overlaps) != 1 {
		t.Fatalf("应有一个冲突区域: %+v", rep.Overlaps)
	}
	if rep.Overlaps[0].Input["x"] != 3 {
		t.Fatalf("冲突最小输入应为 x=3, got %v", rep.Overlaps[0].Input)
	}
}

// 枚举字段与省略字段（不限）。
func TestEnumAndOmittedField(t *testing.T) {
	spec := &Spec{
		Fields: []Field{
			{Name: "tier", Type: FieldEnum, Values: []string{"free", "pro"}},
			intField("x", 0, 2),
		},
		Strategy: StrategyFirst,
		Rules: []Rule{
			rule("r1", map[string]Condition{"tier": {Enum: []string{"free"}}}), // x 不限
			rule("r2", map[string]Condition{"tier": {Enum: []string{"pro"}}, "x": {Range: &IntRange{Min: ptr(0), Max: ptr(1)}}}),
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	rep := Analyze(spec)
	if len(rep.Uncovered) != 1 || rep.Uncovered[0].Input["x"] != 2 {
		t.Fatalf("仅 pro+x=2 未覆盖: %+v", rep.Uncovered)
	}
	if len(rep.Shadowed) != 0 {
		t.Fatalf("不应有遮蔽: %+v", rep.Shadowed)
	}
}

// 单元超限：报告不完整，不给结论。
func TestCellLimitExceeded(t *testing.T) {
	spec := &Spec{
		Fields:    []Field{intField("a", 0, 100), intField("b", 0, 100)},
		Strategy:  StrategyUnique,
		CellLimit: 1,
		Rules: []Rule{
			rule("r1", map[string]Condition{"a": {Range: &IntRange{Min: ptr(1)}}}),
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	rep := Analyze(spec)
	if rep.Complete {
		t.Fatal("超限时不应完成分析")
	}
}

// 校验：未知枚举值、域外区间、空区间定位到规则。
func TestValidation(t *testing.T) {
	base := func() *Spec {
		return &Spec{
			Fields: []Field{
				{Name: "tier", Type: FieldEnum, Values: []string{"free"}},
				intField("x", 0, 10),
			},
			Strategy: StrategyUnique,
		}
	}
	s := base()
	s.Rules = []Rule{rule("bad", map[string]Condition{"tier": {Enum: []string{"gold"}}})}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("未知枚举值应报错并含规则 id: %v", err)
	}
	s = base()
	s.Rules = []Rule{rule("bad", map[string]Condition{"x": {Range: &IntRange{Min: ptr(20)}}})}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("域外区间应报错: %v", err)
	}
	s = base()
	// (5,5) 空区间
	s.Rules = []Rule{rule("bad", map[string]Condition{"x": {Range: &IntRange{Min: ptr(5), MinExclusive: true, Max: ptr(5), MaxExclusive: true}}})}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("空区间应报错: %v", err)
	}
}

// 全覆盖无冲突的干净场景。
func TestCleanSpec(t *testing.T) {
	spec := &Spec{
		Fields:   []Field{intField("x", 0, 3)},
		Strategy: StrategyUnique,
		Rules: []Rule{
			rule("r1", map[string]Condition{"x": {Range: &IntRange{Max: ptr(1)}}}),
			rule("r2", map[string]Condition{"x": {Range: &IntRange{Min: ptr(2)}}}),
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	rep := Analyze(spec)
	if !rep.Complete || len(rep.Uncovered) != 0 || len(rep.Overlaps) != 0 {
		t.Fatalf("应完全干净: %+v", rep)
	}
}
