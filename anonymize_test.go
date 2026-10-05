package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const testHier = `{"hierarchies":{
  "age": {"a1":"young","a2":"young","a3":"old","a4":"old","young":"*","old":"*"},
  "city": {"bj":"north","tj":"north","sh":"south","sz":"south","north":"*","south":"*"}
}}`

const testConfig = `{"k":2,"l":2,"budget":10,"weights":{"age":1,"city":1},"suppressionCost":3}`

const testCSV = `id,age,city,disease
r1,a1,bj,flu
r2,a2,tj,cold
r3,a3,sh,flu
r4,a4,sz,cold
`

func loadAll(t *testing.T, csvContent, hierContent, cfgContent string) (*Dataset, map[string]*Hierarchy, *Config) {
	t.Helper()
	hier, err := LoadHierarchies(writeTemp(t, "h.json", hierContent))
	if err != nil {
		t.Fatalf("层级加载失败: %v", err)
	}
	ds, err := LoadDataset(writeTemp(t, "d.csv", csvContent), hier)
	if err != nil {
		t.Fatalf("数据加载失败: %v", err)
	}
	cfg, err := LoadConfig(writeTemp(t, "c.json", cfgContent), ds.QICols)
	if err != nil {
		t.Fatalf("配置加载失败: %v", err)
	}
	return ds, hier, cfg
}

func TestSearchBestBasic(t *testing.T) {
	ds, hier, cfg := loadAll(t, testCSV, testHier, testConfig)
	best, err := SearchBest(ds, hier, cfg)
	if err != nil {
		t.Fatalf("应存在可行方案: %v", err)
	}
	if !best.Feasible || best.Kept != 4 || best.Suppressed != 0 {
		t.Fatalf("方案异常: %+v", best)
	}
	// 步数 (0,0) 下每组仅 1 人，需泛化；最优应为总步数最小的可行方案。
	if best.Steps[0]+best.Steps[1] == 0 {
		t.Fatalf("步数为零不应满足 k=2: %v", best.Steps)
	}
}

func TestInsufficientSensitiveDiversity(t *testing.T) {
	// 两条记录同组但敏感值相同，l=2 不满足，整组被抑制。
	csvData := `id,age,city,disease
r1,a1,bj,flu
r2,a2,bj,flu
`
	ds, hier, cfg := loadAll(t, csvData, testHier, testConfig)
	ev, err := Evaluate(ds, hier, cfg, []int{1, 1})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Feasible {
		t.Fatalf("敏感值种类不足且保留行数为零，应不可行: %+v", ev)
	}
	if ev.Suppressed != 2 {
		t.Fatalf("应整组抑制 2 条，实际 %d", ev.Suppressed)
	}
	for _, g := range ev.Groups {
		if g.Kept {
			t.Fatalf("敏感值种类不足的组不应保留: %+v", g)
		}
	}
}

func TestCrossHierarchyValueRejected(t *testing.T) {
	// bj 是 city 列的叶值，出现在 age 列应被拒绝。
	csvData := `id,age,city,disease
r1,bj,bj,flu
`
	hier, err := LoadHierarchies(writeTemp(t, "h.json", testHier))
	if err != nil {
		t.Fatal(err)
	}
	_, err = LoadDataset(writeTemp(t, "d.csv", csvData), hier)
	if err == nil || !strings.Contains(err.Error(), "层级树") {
		t.Fatalf("跨列叶值应被拒绝，实际: %v", err)
	}
}

func TestNonTreeHierarchyRejected(t *testing.T) {
	// 环：a->b->c->a，无根。
	bad := `{"hierarchies":{"age":{"a":"b","b":"c","c":"a"}}}`
	if _, err := LoadHierarchies(writeTemp(t, "h.json", bad)); err == nil {
		t.Fatal("环形层级应被拒绝")
	}
	// 多根。
	bad2 := `{"hierarchies":{"age":{"a":"r1","b":"r2"}}}`
	if _, err := LoadHierarchies(writeTemp(t, "h.json", bad2)); err == nil {
		t.Fatal("多根层级应被拒绝")
	}
	// 非叶原始值。
	hier, err := LoadHierarchies(writeTemp(t, "h.json", testHier))
	if err != nil {
		t.Fatal(err)
	}
	csvData := "id,age,city,disease\nr1,young,bj,flu\n"
	if _, err := LoadDataset(writeTemp(t, "d.csv", csvData), hier); err == nil {
		t.Fatal("非叶原始值应被拒绝")
	}
}

func TestBudgetInfeasible(t *testing.T) {
	// 每组仅 1 条记录且敏感值种类不足，预算 0 时无可行方案。
	csvData := `id,age,city,disease
r1,a1,bj,flu
r2,a2,tj,flu
`
	cfg := `{"k":3,"l":2,"budget":0,"weights":{"age":1,"city":1},"suppressionCost":1}`
	ds, hier, c := loadAll(t, csvData, testHier, cfg)
	if _, err := SearchBest(ds, hier, c); err == nil {
		t.Fatal("预算无解时应明确失败")
	}
}

func TestInputReorderKeepsSelection(t *testing.T) {
	reordered := `id,age,city,disease
r4,a4,sz,cold
r2,a2,tj,cold
r3,a3,sh,flu
r1,a1,bj,flu
`
	_, hier1, cfg1 := loadAll(t, testCSV, testHier, testConfig)
	ds1, _, _ := loadAll(t, testCSV, testHier, testConfig)
	ds2, hier2, cfg2 := loadAll(t, reordered, testHier, testConfig)
	best1, err := SearchBest(ds1, hier1, cfg1)
	if err != nil {
		t.Fatal(err)
	}
	best2, err := SearchBest(ds2, hier2, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if best1.TotalCost != best2.TotalCost || best1.Suppressed != best2.Suppressed {
		t.Fatalf("输入重排改变了方案: %+v vs %+v", best1, best2)
	}
	for i := range best1.Steps {
		if best1.Steps[i] != best2.Steps[i] {
			t.Fatalf("输入重排改变了步数: %v vs %v", best1.Steps, best2.Steps)
		}
	}
}

func TestDuplicateIDAndMissingValue(t *testing.T) {
	dup := "id,age,city,disease\nr1,a1,bj,flu\nr1,a2,tj,cold\n"
	hier, err := LoadHierarchies(writeTemp(t, "h.json", testHier))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDataset(writeTemp(t, "d.csv", dup), hier); err == nil {
		t.Fatal("重复标识应被拒绝")
	}
	missing := "id,age,city,disease\nr1,a1,,flu\n"
	if _, err := LoadDataset(writeTemp(t, "d.csv", missing), hier); err == nil {
		t.Fatal("缺失值应被拒绝")
	}
}
