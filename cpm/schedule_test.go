package cpm

import (
	"strings"
	"testing"
)

// 两条并行分支 A-C 与 B-C 等长，二者都是关键路径；
// 只能输出一条沿真实依赖的路径，且选字典序最小的标识序列。
func TestParallelCriticalBranches(t *testing.T) {
	p := &Project{
		Budget: 0,
		Activities: []Activity{
			{ID: "A", Duration: 5},
			{ID: "B", Duration: 5},
			{ID: "C", Duration: 2, Predecessors: []string{"A", "B"}},
		},
	}
	s, err := ComputeSchedule(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.ProjectFinish != 7 {
		t.Fatalf("完工日 = %d, 期望 7", s.ProjectFinish)
	}
	for _, id := range []string{"A", "B", "C"} {
		if !s.Results[id].Critical {
			t.Errorf("活动 %s 应为关键活动", id)
		}
	}
	want := []string{"A", "C"}
	if !equalSlice(s.CriticalPath, want) {
		t.Fatalf("关键路径 = %v, 期望 %v", s.CriticalPath, want)
	}

	// 缩短 B 后，A-C 成为唯一关键路径，B 出现时差。
	s2, err := ComputeSchedule(p, map[string]int{"B": 3})
	if err != nil {
		t.Fatal(err)
	}
	if s2.Results["B"].TotalFloat != 2 {
		t.Fatalf("B 时差 = %d, 期望 2", s2.Results["B"].TotalFloat)
	}
	if !equalSlice(s2.CriticalPath, []string{"A", "C"}) {
		t.Fatalf("关键路径 = %v, 期望 [A C]", s2.CriticalPath)
	}
}

// 零工期里程碑与零工期活动链。
func TestZeroDurationMilestones(t *testing.T) {
	p := &Project{
		Budget: 0,
		Activities: []Activity{
			{ID: "M0", Duration: 0}, // 起始里程碑
			{ID: "A", Duration: 4, Predecessors: []string{"M0"}},
			{ID: "M1", Duration: 0, Predecessors: []string{"A"}}, // 完成里程碑
			{ID: "B", Duration: 3, Predecessors: []string{"M1"}},
		},
	}
	s, err := ComputeSchedule(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.ProjectFinish != 7 {
		t.Fatalf("完工日 = %d, 期望 7", s.ProjectFinish)
	}
	want := []string{"M0", "A", "M1", "B"}
	if !equalSlice(s.CriticalPath, want) {
		t.Fatalf("关键路径 = %v, 期望 %v", s.CriticalPath, want)
	}
	if s.Results["M1"].EarlyStart != s.Results["M1"].EarlyFinish {
		t.Fatal("零工期里程碑起止时间应相同")
	}
}

// 多个无前置活动都从第 0 天开始。
func TestMultipleStartsAtDayZero(t *testing.T) {
	p := &Project{
		Budget: 0,
		Activities: []Activity{
			{ID: "X", Duration: 3},
			{ID: "Y", Duration: 9},
		},
	}
	s, err := ComputeSchedule(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Results["X"].EarlyStart != 0 || s.Results["Y"].EarlyStart != 0 {
		t.Fatal("无前置活动应从第 0 天开始")
	}
	if s.ProjectFinish != 9 {
		t.Fatalf("共用终点完工日 = %d, 期望 9", s.ProjectFinish)
	}
	if s.Results["X"].TotalFloat != 6 {
		t.Fatalf("X 时差 = %d, 期望 6", s.Results["X"].TotalFloat)
	}
	if !equalSlice(s.CriticalPath, []string{"Y"}) {
		t.Fatalf("关键路径 = %v, 期望 [Y]", s.CriticalPath)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		p    *Project
		want string
	}{
		{"空集合", &Project{}, "不能为空"},
		{"标识重复", &Project{Activities: []Activity{
			{ID: "A"}, {ID: "A"},
		}}, "重复"},
		{"未知依赖", &Project{Activities: []Activity{
			{ID: "A", Predecessors: []string{"Z"}},
		}}, "未知活动"},
		{"环", &Project{Activities: []Activity{
			{ID: "A", Predecessors: []string{"B"}},
			{ID: "B", Predecessors: []string{"A"}},
		}}, "环"},
		{"负工期", &Project{Activities: []Activity{
			{ID: "A", Duration: -1},
		}}, "非负"},
		{"负预算", &Project{Budget: -1, Activities: []Activity{
			{ID: "A"},
		}}, "预算"},
		{"压缩未变短", &Project{Activities: []Activity{
			{ID: "A", Duration: 5, Crash: &CrashOption{ShorterDuration: 5, Cost: 1}},
		}}, "严格短于"},
		{"压缩成本非正", &Project{Activities: []Activity{
			{ID: "A", Duration: 5, Crash: &CrashOption{ShorterDuration: 3, Cost: 0}},
		}}, "正整数"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if err == nil {
				_, err = ComputeSchedule(tc.p, nil)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望错误含 %q, 实际 %v", tc.want, err)
			}
		})
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
