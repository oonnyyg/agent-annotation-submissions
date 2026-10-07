package main

import (
	"strings"
	"testing"
)

func testConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := LoadConfig(strings.NewReader(`{
	  "sections": ["A","B","C","D","E"],
	  "switches": {"W1":"normal","W2":"normal"},
	  "routes": [
	    {"id":"R1","sections":["A","B","C"],"switches":{"W1":"normal"}},
	    {"id":"R2","sections":["D","B","E"],"switches":{"W1":"normal"}},
	    {"id":"R3","sections":["C","D"],"switches":{"W1":"reverse"}},
	    {"id":"R4","sections":["E"],"switches":{"W2":"reverse"}}
	  ]
	}`))
	if err != nil {
		t.Fatalf("配置应合法: %v", err)
	}
	return cfg
}

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"重复区段", `{"sections":["A"],"routes":[{"id":"R","sections":["A","A"]}]}`},
		{"未知区段", `{"sections":["A"],"routes":[{"id":"R","sections":["B"]}]}`},
		{"未知道岔", `{"sections":["A"],"routes":[{"id":"R","sections":["A"],"switches":{"W":"normal"}}]}`},
		{"非法初始位置", `{"sections":["A"],"switches":{"W":"sideways"},"routes":[{"id":"R","sections":["A"]}]}`},
		{"空进路", `{"sections":["A"],"routes":[{"id":"R","sections":[]}]}`},
		{"重复进路号", `{"sections":["A"],"routes":[{"id":"R","sections":["A"]},{"id":"R","sections":["A"]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadConfig(strings.NewReader(tc.json)); err == nil {
				t.Fatalf("非法配置应被拒绝: %s", tc.json)
			}
		})
	}
}

func TestSharedSectionConflict(t *testing.T) {
	il := NewInterlocking(testConfig(t))
	if err := il.RequestRoute("R1"); err != nil {
		t.Fatalf("R1 应申请成功: %v", err)
	}
	if err := il.RequestRoute("R2"); err == nil {
		t.Fatal("R2 与 R1 共享区段 B，应冲突")
	}
	if _, locked := il.sectionLocks["D"]; locked {
		t.Fatal("R2 申请失败不得遗留对区段 D 的部分锁")
	}
	if il.switchHolders["W1"]["R2"] {
		t.Fatal("R2 申请失败不得遗留道岔持有")
	}
}

func TestSwitchPositionConflictAndSharedHold(t *testing.T) {
	il := NewInterlocking(testConfig(t))
	if err := il.RequestRoute("R1"); err != nil {
		t.Fatalf("R1 应申请成功: %v", err)
	}
	if err := il.RequestRoute("R3"); err == nil {
		t.Fatal("R3 要求 W1=reverse，与 R1 的 normal 冲突，应拒绝")
	}
	if il.switchPos["W1"] != Normal {
		t.Fatal("R3 失败后 W1 位置不得被改变")
	}
	if err := il.CancelRoute("R1"); err != nil {
		t.Fatalf("取消 R1: %v", err)
	}
	if err := il.RequestRoute("R3"); err != nil {
		t.Fatalf("R1 释放后 R3 应成功: %v", err)
	}
	if il.switchPos["W1"] != Reverse {
		t.Fatal("R3 成功后 W1 应被设置为 reverse")
	}
}

func TestSharedSwitchSamePositionAllowed(t *testing.T) {
	cfg, err := LoadConfig(strings.NewReader(`{
	  "sections": ["A","B"],
	  "switches": {"W":"normal"},
	  "routes": [
	    {"id":"RA","sections":["A"],"switches":{"W":"normal"}},
	    {"id":"RB","sections":["B"],"switches":{"W":"normal"}}
	  ]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	il := NewInterlocking(cfg)
	if err := il.RequestRoute("RA"); err != nil {
		t.Fatal(err)
	}
	if err := il.RequestRoute("RB"); err != nil {
		t.Fatalf("同一道岔要求相同位置应允许共享持有: %v", err)
	}
	if err := il.SetSwitch("W", Reverse); err == nil {
		t.Fatal("道岔仍被持有，手动转位应被拒绝")
	}
	if il.switchPos["W"] != Normal {
		t.Fatal("被拒绝的手动转位不得改变位置")
	}
	if err := il.CancelRoute("RA"); err != nil {
		t.Fatal(err)
	}
	if !il.switchHolders["W"]["RB"] {
		t.Fatal("释放 RA 不得清掉 RB 的持有关系")
	}
	if err := il.SetSwitch("W", Reverse); err == nil {
		t.Fatal("RB 仍持有，手动转位仍应被拒绝")
	}
	if err := il.CancelRoute("RB"); err != nil {
		t.Fatal(err)
	}
	if err := il.SetSwitch("W", Reverse); err != nil {
		t.Fatalf("全部释放后手动转位应成功: %v", err)
	}
}

func TestTrainProgressAndRelease(t *testing.T) {
	il := NewInterlocking(testConfig(t))
	if err := il.Enter("T1", "R1"); err == nil {
		t.Fatal("未申请进路，进入应被拒绝")
	}
	if err := il.RequestRoute("R1"); err != nil {
		t.Fatal(err)
	}
	if err := il.RequestRoute("R4"); err != nil {
		t.Fatal(err)
	}
	if err := il.Enter("T1", "R1"); err != nil {
		t.Fatal(err)
	}
	if il.occupied["A"] != "T1" {
		t.Fatal("T1 应占用首区段 A")
	}
	if err := il.Enter("T1", "R4"); err == nil {
		t.Fatal("同一列车同时进入两条进路应被拒绝")
	}
	if err := il.CancelRoute("R1"); err == nil {
		t.Fatal("列车已进入，取消应被拒绝")
	}
	if _, locked := il.sectionLocks["A"]; !locked {
		t.Fatal("被拒绝的取消不得解除区段锁")
	}
	if err := il.Advance("T1"); err != nil {
		t.Fatal(err)
	}
	if il.occupied["A"] != "" || il.occupied["B"] != "T1" {
		t.Fatal("推进应清空前区段并占用下一区段")
	}
	if err := il.Advance("T2"); err == nil {
		t.Fatal("不在进路上的列车推进应被拒绝")
	}
	if err := il.Advance("T1"); err != nil {
		t.Fatal(err)
	}
	if il.occupied["C"] != "T1" {
		t.Fatal("T1 应占用末区段 C")
	}
	if _, locked := il.sectionLocks["C"]; !locked {
		t.Fatal("末区段退出前锁必须保留")
	}
	if err := il.Advance("T1"); err != nil {
		t.Fatal(err)
	}
	if len(il.sectionLocks) != 1 || il.sectionLocks["E"] != "R4" {
		t.Fatal("列车退出末区段后 R1 应整体释放，仅保留 R4 的锁")
	}
	if il.switchHolders["W1"]["R1"] {
		t.Fatal("R1 释放后不得残留道岔持有")
	}
	if len(il.occupied) != 0 {
		t.Fatal("列车完全退出后不应有占用")
	}
}

func TestCancelBeforeEnter(t *testing.T) {
	il := NewInterlocking(testConfig(t))
	if err := il.RequestRoute("R1"); err != nil {
		t.Fatal(err)
	}
	if err := il.CancelRoute("R1"); err != nil {
		t.Fatalf("列车未进入时取消应成功: %v", err)
	}
	if len(il.sectionLocks) != 0 || len(il.switchHolders) != 0 {
		t.Fatal("取消后不得残留锁或持有")
	}
	if err := il.CancelRoute("R1"); err == nil {
		t.Fatal("重复取消应被拒绝")
	}
}
