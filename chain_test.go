package main

import (
	"math"
	"testing"
)

func TestNonIdentityBase(t *testing.T) {
	c := planar2R()
	// Base rotated 90 deg about z and lifted 1 m.
	c.Base = PoseJSON{
		Rotation:    Mat3{{0, -1, 0}, {1, 0, 0}, {0, 0, 1}},
		Translation: Vec3{0, 0, 1},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	_, tool, err := c.ForwardKinematics([]float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	// Link direction (world x) rotated to world y, plus base lift.
	vecNear(t, tool.P, Vec3{0, 2, 1}, 1e-12, "tool origin with rotated base")
}

func TestToolOffsetFollowsEndRotation(t *testing.T) {
	c := &Chain{
		Joints: []Joint{
			{Name: "j1", Type: JointRevolute, A: 1, Limit: [2]float64{-math.Pi, math.Pi}},
		},
		Base: identPose(),
		Tool: PoseJSON{
			Rotation:    Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}},
			Translation: Vec3{0.1, 0, 0},
		},
	}
	_, tool, err := c.ForwardKinematics([]float64{math.Pi / 2})
	if err != nil {
		t.Fatal(err)
	}
	// End frame x-axis points along world y, so the tool offset lands on y.
	vecNear(t, tool.P, Vec3{0, 1.1, 0}, 1e-12, "tool origin with rotated offset")
	// Fixed tool offset must affect linear velocity rows.
	jac, _, err := c.GeometricJacobian([]float64{math.Pi / 2})
	if err != nil {
		t.Fatal(err)
	}
	vecNear(t, Vec3{jac.Cols[0][0], jac.Cols[0][1], jac.Cols[0][2]}, Vec3{-1.1, 0, 0}, 1e-12, "Jv with tool offset")
}

func TestLimitViolationRejected(t *testing.T) {
	c := planar2R()
	_, _, err := c.ForwardKinematics([]float64{0, math.Pi + 0.1})
	if err == nil {
		t.Fatal("expected limit violation error")
	}
	res := processSample(c, Sample{Name: "bad", Q: []float64{0, math.Pi + 0.1}, Qd: []float64{0, 0}})
	if res.OK || res.Tool != nil || res.Jacobian != nil {
		t.Fatalf("rejected sample must not carry pose data: %+v", res)
	}
}

func TestInvalidRotationsRejected(t *testing.T) {
	c := planar2R()
	c.Base.Rotation = Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, -1}} // det = -1
	if err := c.Validate(); err == nil {
		t.Fatal("expected determinant error")
	}
	c = planar2R()
	c.Tool.Rotation = Mat3{{1, 0, 0}, {0, 2, 0}, {0, 0, 1}} // not orthonormal
	if err := c.Validate(); err == nil {
		t.Fatal("expected orthonormality error")
	}
}

func TestNumericalJacobianCheck(t *testing.T) {
	c := &Chain{
		Joints: []Joint{
			{Name: "j1", Type: JointRevolute, A: 0.4, Alpha: math.Pi / 2, Limit: [2]float64{-1, 1}},
			{Name: "j2", Type: JointPrismatic, D: 0.1, A: 0.2, Limit: [2]float64{0, 0.3}},
			{Name: "j3", Type: JointRevolute, A: 0.15, Offset: 0.1, Limit: [2]float64{-2, 2}},
		},
		Base: identPose(),
		Tool: PoseJSON{
			Rotation:    Mat3{{1, 0, 0}, {0, 0, -1}, {0, 1, 0}},
			Translation: Vec3{0.05, 0.02, -0.03},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	// Interior sample: all central differences.
	q := []float64{0.3, 0.15, -0.4}
	jac, _, err := c.GeometricJacobian(q)
	if err != nil {
		t.Fatal(err)
	}
	checks, err := c.NumericalPositionJacobian(q, jac)
	if err != nil {
		t.Fatal(err)
	}
	for _, chk := range checks {
		if chk.Scheme != "central" {
			t.Fatalf("joint %s: expected central scheme, got %s", chk.Joint, chk.Scheme)
		}
		if chk.MaxError > 1e-5 {
			t.Fatalf("joint %s: max error %v too large", chk.Joint, chk.MaxError)
		}
	}
	// Boundary sample: j1 at max, j2 at min -> one-sided, never out of limits.
	qb := []float64{1, 0, 0.5}
	jacb, _, err := c.GeometricJacobian(qb)
	if err != nil {
		t.Fatal(err)
	}
	checksb, err := c.NumericalPositionJacobian(qb, jacb)
	if err != nil {
		t.Fatal(err)
	}
	if checksb[0].Scheme != "backward" {
		t.Fatalf("j1 at max: expected backward scheme, got %s", checksb[0].Scheme)
	}
	if checksb[1].Scheme != "forward" {
		t.Fatalf("j2 at min: expected forward scheme, got %s", checksb[1].Scheme)
	}
	for _, chk := range checksb {
		if chk.MaxError > 1e-4 {
			t.Fatalf("joint %s: boundary max error %v too large", chk.Joint, chk.MaxError)
		}
	}
}
