package main

import (
	"strings"
	"testing"
)

func load(t *testing.T, lines ...string) map[string][]*Snapshot {
	t.Helper()
	m, err := LoadSnapshots(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("LoadSnapshots: %v", err)
	}
	return m
}

func TestUnorderedInputAndMerge(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":200,"boot":"b1","bounds":[0.5,1],"counts":[6,9,11],"sum":7}`,
		`{"instance":"b","ts":100,"boot":"b1","bounds":[0.5,1],"counts":[1,2,3],"sum":1.5}`,
		`{"instance":"a","ts":100,"boot":"b1","bounds":[0.5,1],"counts":[1,2,4],"sum":2}`,
		`{"instance":"b","ts":200,"boot":"b1","bounds":[0.5,1],"counts":[2,5,6],"sum":4}`,
	)
	res, err := ComputeWindow(m, []string{"a", "b"}, 100, 200)
	if err != nil {
		t.Fatalf("ComputeWindow: %v", err)
	}
	// a: 累计差 (5,7,7) → 每桶 (5,2,0) sum 5; b: 累计差 (1,3,3) → 每桶 (1,2,0) sum 2.5
	want := []uint64{6, 4, 0}
	for i := range want {
		if res.BucketDiff[i] != want[i] {
			t.Fatalf("bucket %d = %d, want %d", i, res.BucketDiff[i], want[i])
		}
	}
	if res.Total != 10 || res.SumDiff != 7.5 {
		t.Fatalf("total=%d sum=%v, want 10/7.5", res.Total, res.SumDiff)
	}
	avg, ok := res.AvgLatency()
	if !ok || avg != 0.75 {
		t.Fatalf("avg=%v ok=%v, want 0.75", avg, ok)
	}
	if rps := res.RequestsPerSecond(); rps != 0.1 {
		t.Fatalf("rps=%v, want 0.1", rps)
	}
	if len(res.Gaps) != 0 {
		t.Fatalf("unexpected gaps: %+v", res.Gaps)
	}
}

func TestRestartAcrossWindow(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":100,"boot":"b1","bounds":[1],"counts":[5,10],"sum":4}`,
		`{"instance":"a","ts":150,"boot":"b1","bounds":[1],"counts":[8,13],"sum":6}`,
		`{"instance":"a","ts":200,"boot":"b2","bounds":[1],"counts":[3,7],"sum":2}`,
	)
	res, err := ComputeWindow(m, []string{"a"}, 100, 200)
	if err != nil {
		t.Fatalf("ComputeWindow: %v", err)
	}
	// 100→150 同代号累计差 (3,3) sum 2；150→200 重启，只计新快照累计 (3,7) sum 2
	if res.Total != 10 || res.SumDiff != 4 {
		t.Fatalf("total=%d sum=%v, want 10/4", res.Total, res.SumDiff)
	}
	if len(res.Gaps) != 1 || res.Gaps[0].FromTs != 150 || res.Gaps[0].ToTs != 200 {
		t.Fatalf("gaps=%+v", res.Gaps)
	}
}

func TestRestartNegativeDiffRejected(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":100,"boot":"b1","bounds":[1],"counts":[5,10],"sum":4}`,
		`{"instance":"a","ts":200,"boot":"b1","bounds":[1],"counts":[4,10],"sum":5}`,
	)
	if _, err := ComputeWindow(m, []string{"a"}, 100, 200); err == nil {
		t.Fatal("expected error for decreasing bucket count")
	}
}

func TestDuplicateAndValidation(t *testing.T) {
	_, err := LoadSnapshots(strings.NewReader(
		`{"instance":"a","ts":100,"boot":"b1","bounds":[1],"counts":[1,2],"sum":1}` + "\n" +
			`{"instance":"a","ts":100,"boot":"b1","bounds":[1],"counts":[1,2],"sum":1}`))
	if err == nil {
		t.Fatal("expected duplicate rejection")
	}
	bad := []string{
		`{"instance":"a","ts":1,"boot":"b","bounds":[],"counts":[0],"sum":0}`,        // 无有限桶
		`{"instance":"a","ts":1,"boot":"b","bounds":[2,1],"counts":[0,0,0],"sum":0}`, // 非递增
		`{"instance":"a","ts":1,"boot":"b","bounds":[1],"counts":[2,1],"sum":1}`,     // 计数下降
		`{"instance":"a","ts":1,"boot":"b","bounds":[1],"counts":[0,0],"sum":0.5}`,   // 零总数非零耗时
		`{"instance":"a","ts":1,"boot":"b","bounds":[1],"counts":[0,0,0],"sum":0}`,   // 长度不符
	}
	for _, line := range bad {
		if _, err := LoadSnapshots(strings.NewReader(line)); err == nil {
			t.Fatalf("expected validation error for %s", line)
		}
	}
}

func TestMissingEndpointAndBoundsMismatch(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":100,"boot":"b1","bounds":[1],"counts":[1,2],"sum":1}`,
		`{"instance":"a","ts":200,"boot":"b1","bounds":[1],"counts":[2,4],"sum":2}`,
		`{"instance":"b","ts":100,"boot":"b1","bounds":[1],"counts":[1,1],"sum":0.5}`,
	)
	if _, err := ComputeWindow(m, []string{"a", "b"}, 100, 200); err == nil {
		t.Fatal("expected missing endpoint error")
	}
	m2 := load(t,
		`{"instance":"a","ts":100,"boot":"b1","bounds":[1],"counts":[1,2],"sum":1}`,
		`{"instance":"a","ts":200,"boot":"b1","bounds":[1,2],"counts":[1,2,4],"sum":2}`,
	)
	if _, err := ComputeWindow(m2, []string{"a"}, 100, 200); err == nil {
		t.Fatal("expected bounds mismatch error")
	}
}

func TestQuantilesAndInfBucket(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":100,"boot":"b1","bounds":[1,2],"counts":[0,0,0],"sum":0}`,
		`{"instance":"a","ts":200,"boot":"b1","bounds":[1,2],"counts":[4,8,10],"sum":9}`,
	)
	res, err := ComputeWindow(m, []string{"a"}, 100, 200)
	if err != nil {
		t.Fatalf("ComputeWindow: %v", err)
	}
	// 桶分布: [0,1]:4, (1,2]:4, (2,+Inf):2
	cases := []struct {
		q    float64
		want float64
	}{
		{0, 0},
		{0.2, 0.5},  // target=2，首桶内 0 + 2/4*1
		{0.5, 1.25}, // target=5，第二桶 1 + 1/4*1
		{1, 2},      // 落入无穷桶 → 最后有限上界
		{0.95, 2},
	}
	for _, c := range cases {
		v, ok, err := res.Quantile(c.q)
		if err != nil || !ok || v != c.want {
			t.Fatalf("q=%v: v=%v ok=%v err=%v, want %v", c.q, v, ok, err, c.want)
		}
	}
	if _, _, err := res.Quantile(1.5); err == nil {
		t.Fatal("expected quantile range error")
	}
}

func TestZeroRequests(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":100,"boot":"b1","bounds":[1],"counts":[0,0],"sum":0}`,
		`{"instance":"a","ts":200,"boot":"b1","bounds":[1],"counts":[0,0],"sum":0}`,
	)
	res, err := ComputeWindow(m, []string{"a"}, 100, 200)
	if err != nil {
		t.Fatalf("ComputeWindow: %v", err)
	}
	if _, ok := res.AvgLatency(); ok {
		t.Fatal("zero-request avg should be empty")
	}
	if _, ok, err := res.Quantile(0.5); err != nil || ok {
		t.Fatal("zero-request quantile should be empty")
	}
	rep, err := buildReport(res, []string{"a"}, []float64{0.5})
	if err != nil {
		t.Fatal(err)
	}
	if rep.AvgLatency != nil || rep.Quantiles[0].Value != nil {
		t.Fatal("report should contain null avg/quantile")
	}
}
