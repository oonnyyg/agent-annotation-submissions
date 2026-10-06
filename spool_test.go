package main

import (
	"reflect"
	"testing"
)

func TestParseRangesOverlapAndOrder(t *testing.T) {
	pages, err := parseRanges("3-5,1,2-4", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(pages, want) {
		t.Fatalf("got %v want %v", pages, want)
	}
}

func TestParseRangesInvalid(t *testing.T) {
	for _, spec := range []string{"", "0", "2-1", "a", "1-", "1-2-3", "6", "1,xyz"} {
		if _, err := parseRanges(spec, 5); err == nil {
			t.Fatalf("spec %q should be rejected", spec)
		}
	}
}

func TestBuildSheetsDuplexOddPages(t *testing.T) {
	sheets, err := buildSheets([]int{1, 2, 3, 4, 5}, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []Sheet{{Front: 1, Back: 2}, {Front: 3, Back: 4}, {Front: 5, Back: 0}}
	if !reflect.DeepEqual(sheets, want) {
		t.Fatalf("got %+v want %+v", sheets, want)
	}
}

func TestBuildSheetsMultiCopyEachStartsOnFront(t *testing.T) {
	// 3 页双面 2 份：每份 2 张，第二份重新从正面开始。
	sheets, err := buildSheets([]int{1, 2, 3}, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []Sheet{
		{Front: 1, Back: 2}, {Front: 3, Back: 0},
		{Front: 1, Back: 2}, {Front: 3, Back: 0},
	}
	if !reflect.DeepEqual(sheets, want) {
		t.Fatalf("got %+v want %+v", sheets, want)
	}
}

func TestBuildSheetsSimplex(t *testing.T) {
	sheets, err := buildSheets([]int{2, 4}, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []Sheet{{Front: 2}, {Front: 4}, {Front: 2}, {Front: 4}}
	if !reflect.DeepEqual(sheets, want) {
		t.Fatalf("got %+v want %+v", sheets, want)
	}
}

func setup(t *testing.T) *Spool {
	t.Helper()
	s := NewSpool()
	if err := s.Register("alice", 10); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSubmitReservesAndQuotaEnforced(t *testing.T) {
	s := setup(t)
	if _, err := s.Submit("alice", "j1", 5, "1-5", 2, true); err != nil {
		t.Fatal(err)
	}
	u := s.Users["alice"]
	if u.Reserved != 6 || u.Used != 0 {
		t.Fatalf("reserved=%d used=%d, want 6/0", u.Reserved, u.Used)
	}
	// 剩余 4 张，再要 5 张（单面 5 页）必须拒绝。
	if _, err := s.Submit("alice", "j2", 5, "1-5", 1, false); err == nil {
		t.Fatal("expected quota rejection")
	}
	if u.Reserved != 6 {
		t.Fatal("failed submit must not change reservation")
	}
	if _, err := s.Submit("bob", "j3", 1, "1", 1, false); err == nil {
		t.Fatal("unregistered user must be rejected")
	}
}

func TestConfirmFlowAndIdempotency(t *testing.T) {
	s := setup(t)
	if _, err := s.Submit("alice", "j1", 3, "1-3", 1, true); err != nil {
		t.Fatal(err)
	}
	// 跳过第 0 张直接确认第 1 张必须拒绝。
	if err := s.Confirm("j1", 1); err == nil {
		t.Fatal("skip confirm must be rejected")
	}
	if err := s.Confirm("j1", 0); err != nil {
		t.Fatal(err)
	}
	// 重复确认同一张：幂等，不重复扣纸。
	if err := s.Confirm("j1", 0); err != nil {
		t.Fatal(err)
	}
	u := s.Users["alice"]
	if u.Used != 1 || u.Reserved != 1 {
		t.Fatalf("used=%d reserved=%d, want 1/1", u.Used, u.Reserved)
	}
	if err := s.Confirm("j1", 1); err != nil {
		t.Fatal(err)
	}
	j := s.findJob("j1")
	if j.Status != StatusDone {
		t.Fatalf("status=%s want done", j.Status)
	}
	if err := s.Confirm("j1", 1); err == nil {
		t.Fatal("confirm on finished job must be rejected")
	}
}

func TestPauseDoesNotBlockQueue(t *testing.T) {
	s := setup(t)
	if _, err := s.Submit("alice", "j1", 2, "1-2", 1, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit("alice", "j2", 3, "1-3", 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause("j1"); err != nil {
		t.Fatal(err)
	}
	// 暂挂的 j1 不阻塞 j2；j1 不是活动作业，确认被拒绝。
	if err := s.Confirm("j1", 0); err == nil {
		t.Fatal("paused job must not be confirmable")
	}
	if err := s.Confirm("j2", 0); err != nil {
		t.Fatal(err)
	}
	// 已开始打印的作业不能暂挂。
	if err := s.Pause("j2"); err == nil {
		t.Fatal("started job must not be pausable")
	}
	if err := s.Resume("j1"); err != nil {
		t.Fatal(err)
	}
	// j2 先提交且未完成，仍是活动作业。
	if err := s.Confirm("j1", 0); err == nil {
		t.Fatal("j2 is still ahead of j1")
	}
	if err := s.Confirm("j2", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm("j1", 0); err != nil {
		t.Fatal(err)
	}
}

func TestCancelReleasesOnlyUnprinted(t *testing.T) {
	s := setup(t)
	if _, err := s.Submit("alice", "j1", 5, "1-5", 1, true); err != nil {
		t.Fatal(err)
	}
	// 模拟中断：确认 2 张后取消。
	for i := 0; i < 2; i++ {
		if err := s.Confirm("j1", i); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Cancel("j1"); err != nil {
		t.Fatal(err)
	}
	u := s.Users["alice"]
	if u.Used != 2 || u.Reserved != 0 {
		t.Fatalf("used=%d reserved=%d, want 2/0", u.Used, u.Reserved)
	}
	j := s.findJob("j1")
	if j.Status != StatusCanceled || j.Confirmed != 2 {
		t.Fatalf("job history lost: %+v", j)
	}
	// 已结束作业不能确认或再次取消。
	if err := s.Confirm("j1", 2); err == nil {
		t.Fatal("canceled job must not accept confirm")
	}
	if err := s.Cancel("j1"); err == nil {
		t.Fatal("double cancel must be rejected")
	}
	if u.Used != 2 || u.Reserved != 0 {
		t.Fatal("illegal ops must not change quota")
	}
}

func TestPersistenceResume(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/state.json"
	s := NewSpool()
	if err := s.Register("alice", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit("alice", "j1", 5, "1-5", 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm("j1", 0); err != nil {
		t.Fatal(err)
	}
	if err := saveSpool(path, s); err != nil {
		t.Fatal(err)
	}
	// 模拟重启：重新加载，从下一张继续。
	s2, err := loadSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	j := s2.findJob("j1")
	if j == nil || j.Confirmed != 1 || j.Status != StatusPrinting {
		t.Fatalf("bad restored job: %+v", j)
	}
	if err := s2.Confirm("j1", 1); err != nil {
		t.Fatal(err)
	}
	// 重复确认已确认的纸幂等且不重复扣纸。
	if err := s2.Confirm("j1", 0); err != nil {
		t.Fatal(err)
	}
	if u := s2.Users["alice"]; u.Used != 2 || u.Reserved != 1 {
		t.Fatalf("used=%d reserved=%d, want 2/1", u.Used, u.Reserved)
	}
}
