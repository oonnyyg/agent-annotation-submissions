package audit

import (
	"testing"

	"example.com/polygon-region-audit/internal/geom"
)

func pt(x, y int64) geom.Point { return geom.Point{X: x, Y: y} }

func sampleInput() Input {
	return Input{
		Regions: []RegionInput{
			{
				ID: "ok",
				Rings: []geom.Ring{
					{pt(0, 0), pt(10, 0), pt(10, 10), pt(0, 10), pt(0, 0)},
					{pt(2, 2), pt(4, 2), pt(4, 4), pt(2, 4), pt(2, 2)},
				},
			},
			{
				ID: "bad",
				Rings: []geom.Ring{
					{pt(0, 0), pt(10, 0), pt(10, 10), pt(0, 10)}, // not closed
				},
			},
		},
		Points: []PointInput{
			{ID: "p-in", X: 6, Y: 6},
			{ID: "p-hole", X: 3, Y: 3},
			{ID: "p-out", X: 50, Y: 50},
			{ID: "p-outer-edge", X: 0, Y: 5},
			{ID: "p-hole-edge", X: 2, Y: 3},
		},
	}
}

func TestRun(t *testing.T) {
	out := Run(sampleInput())
	if len(out.Regions) != 2 {
		t.Fatalf("got %d region reports", len(out.Regions))
	}
	ok, bad := out.Regions[0], out.Regions[1]
	if !ok.Valid {
		t.Fatalf("valid region rejected: %+v", ok.Errors)
	}
	if ok.Area != 96 {
		t.Fatalf("area = %v, want 96", ok.Area)
	}
	if bad.Valid {
		t.Fatal("invalid region accepted")
	}
	if len(bad.Errors) == 0 || bad.Errors[0].Ring != 0 {
		t.Fatalf("error not located: %+v", bad.Errors)
	}

	want := map[string]string{
		"p-in": LocInside, "p-hole": LocOutside, "p-out": LocOutside,
		"p-outer-edge": LocOuterBoundary, "p-hole-edge": LocHoleBoundary,
	}
	for _, pr := range out.Points {
		if len(pr.Results) != 1 {
			t.Fatalf("%s: invalid region took part in classification: %v", pr.ID, pr.Results)
		}
		if got := pr.Results["ok"]; got != want[pr.ID] {
			t.Errorf("%s: got %q, want %q", pr.ID, got, want[pr.ID])
		}
	}
}

func TestOverlappingRegionsReportedSeparately(t *testing.T) {
	in := Input{
		Regions: []RegionInput{
			{ID: "a", Rings: []geom.Ring{{pt(0, 0), pt(10, 0), pt(10, 10), pt(0, 10), pt(0, 0)}}},
			{ID: "b", Rings: []geom.Ring{{pt(5, 5), pt(15, 5), pt(15, 15), pt(5, 15), pt(5, 5)}}},
		},
		Points: []PointInput{{ID: "p", X: 7, Y: 7}},
	}
	out := Run(in)
	res := out.Points[0].Results
	if res["a"] != LocInside || res["b"] != LocInside {
		t.Fatalf("overlapping regions not reported separately: %v", res)
	}
}
