package main

import "testing"

func reportFor(t *testing.T, counts ...uint64) *Report {
	t.Helper()
	rep, err := MergeWindow(load(t,
		`{"instance":"a","ts":0,"boot":"x","bounds":[1,10],"counts":[0,0,0],"sum":0}`,
		`{"instance":"a","ts":100,"boot":"x","bounds":[1,10],"counts":[`+
			itoa(counts[0])+`,`+itoa(counts[0]+counts[1])+`,`+itoa(counts[0]+counts[1]+counts[2])+`],"sum":50}`,
	), 0, 100)
	if err != nil {
		t.Fatalf("MergeWindow: %v", err)
	}
	return rep
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

func TestQuantileInterpolation(t *testing.T) {
	// Buckets: [0,1]:10, (1,10]:10, (10,+Inf]:0; total 20.
	rep := reportFor(t, 10, 10, 0)
	if v, ok := rep.Quantile(0); !ok || v != 0 {
		t.Errorf("q0 = %v,%v want 0,true", v, ok)
	}
	// q0.5 -> rank 10, last of first bucket -> upper bound 1.
	if v, ok := rep.Quantile(0.5); !ok || v != 1 {
		t.Errorf("q0.5 = %v,%v want 1,true", v, ok)
	}
	// q0.75 -> rank 15, halfway through second bucket: 1 + 0.5*9 = 5.5.
	if v, ok := rep.Quantile(0.75); !ok || v != 5.5 {
		t.Errorf("q0.75 = %v,%v want 5.5,true", v, ok)
	}
}

func TestQuantileInfBucket(t *testing.T) {
	// All mass in +Inf bucket -> last finite bound.
	rep := reportFor(t, 0, 0, 5)
	if v, ok := rep.Quantile(0.9); !ok || v != 10 {
		t.Errorf("q0.9 = %v,%v want 10,true", v, ok)
	}
}

func TestQuantileZeroRequests(t *testing.T) {
	m := load(t,
		`{"instance":"a","ts":0,"boot":"x","bounds":[1],"counts":[0,0],"sum":0}`,
		`{"instance":"a","ts":10,"boot":"x","bounds":[1],"counts":[0,0],"sum":0}`,
	)
	rep, err := MergeWindow(m, 0, 10)
	if err != nil {
		t.Fatalf("MergeWindow: %v", err)
	}
	if _, ok := rep.Quantile(0.5); ok {
		t.Error("expected ok=false for zero requests")
	}
	if _, ok := rep.AvgLatency(); ok {
		t.Error("expected avg undefined for zero requests")
	}
	if rep.RequestsPerSecond() != 0 {
		t.Error("expected rps 0")
	}
}
