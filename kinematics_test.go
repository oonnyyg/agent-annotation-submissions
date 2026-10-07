package main

import (
	"math"
	"testing"
)

func almost(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func vecAlmost(t *testing.T, got, want [3]float64, tol float64, what string) {
	t.Helper()
	for i := 0; i < 3; i++ {
		if !almost(got[i], want[i], tol) {
			t.Errorf("%s: got %v, want %v", what, got, want)
			return
		}
	}
}

// mixedChain has a revolute joint, a prismatic joint, a non-identity base
// (90 deg about z plus translation) and a translated+rotated tool.
func mixedChain() Chain {
	base := dhTransform(math.Pi/2, 0.1, 0, 0) // Rz(90) then Tz(0.1)
	tool := dhTransform(0, 0, 0.05, math.Pi/2)
	return Chain{
		Name: "mixed",
		Joints: []Joint{
			{Name: "shoulder", Type: Revolute, A: 0.3, Min: -math.Pi, Max: math.Pi},
			{Name: "lift", Type: Prismatic, D: 0.2, Min: 0, Max: 0.5},
		},
		Base: base,
		Tool: tool,
	}
}

func TestForwardKinematicsMixedChain(t *testing.T) {
	c := mixedChain()
	frames, err := c.ForwardKinematics([]float64{0, 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	// Joint 0 frame: base * Rz(0)Tz(0)Tx(0.3)Rx(0).
	// Base rotates x->y, so joint-0 origin = (0, 0.3, 0.1).
	vecAlmost(t, frames[0].Trans(), [3]float64{0, 0.3, 0.1}, 1e-12, "joint0 origin")
	// Prismatic joint slides along joint-0 z (world z): +0.2+0.1.
	vecAlmost(t, frames[1].Trans(), [3]float64{0, 0.3, 0.4}, 1e-12, "joint1 origin")
	// Tool: Tx(0.05) along joint-1 x (world y), then Rx(90) rotates nothing
	// about the origin, so tool origin = (0, 0.35, 0.4).
	vecAlmost(t, frames[2].Trans(), [3]float64{0, 0.35, 0.4}, 1e-12, "tool origin")
}

func TestToolOffsetFollowsEndRotation(t *testing.T) {
	// Rotating joint 0 by 90 deg must swing the tool tip in the world frame;
	// a tool offset wrongly added in world axes would not move.
	c := mixedChain()
	p0, err := c.ToolPose([]float64{0, 0.1})
	if err != nil {
		t.Fatal(err)
	}
	p1, err := c.ToolPose([]float64{math.Pi / 2, 0.1})
	if err != nil {
		t.Fatal(err)
	}
	// After base Rz(90) and joint Rz(90), joint x points along world -x.
	vecAlmost(t, p1.Trans(), [3]float64{-0.35, 0, 0.4}, 1e-12, "rotated tool origin")
	if d := p0.Trans()[1] - p1.Trans()[1]; math.Abs(d) < 0.1 {
		t.Errorf("tool offset did not follow end rotation: %v vs %v", p0.Trans(), p1.Trans())
	}
}

func TestLimitRejection(t *testing.T) {
	c := mixedChain()
	if _, err := c.ForwardKinematics([]float64{0, 0.6}); err == nil {
		t.Error("expected out-of-limit prismatic sample to be rejected")
	}
	if _, err := c.ForwardKinematics([]float64{0, -0.1}); err == nil {
		t.Error("expected negative prismatic sample to be rejected")
	}
	// Offset shifts the allowed raw range.
	c.Joints[1].Offset = 0.1
	if _, err := c.ForwardKinematics([]float64{0, 0.45}); err == nil {
		t.Error("expected offset-adjusted limit violation to be rejected")
	}
	if _, err := c.ForwardKinematics([]float64{0, 0.4}); err != nil {
		t.Errorf("position at adjusted limit should pass: %v", err)
	}
}

func TestInvalidTransformsRejected(t *testing.T) {
	c := mixedChain()
	c.Tool[0][0] = 2 // scaled, non-orthonormal
	if err := c.Validate(); err == nil {
		t.Error("expected non-orthonormal tool rotation to be rejected")
	}
	c = mixedChain()
	c.Base[0][0] = -1 // reflection, det = -1
	if err := c.Validate(); err == nil {
		t.Error("expected determinant -1 base rotation to be rejected")
	}
}

func TestToolVelocity(t *testing.T) {
	c := mixedChain()
	pos := []float64{0, 0.1}
	// Unit prismatic velocity: tool moves along world z at 1 m/s.
	lin, ang, err := c.ToolVelocity(pos, []float64{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	vecAlmost(t, lin, [3]float64{0, 0, 1}, 1e-12, "prismatic linear velocity")
	vecAlmost(t, ang, [3]float64{0, 0, 0}, 1e-12, "prismatic angular velocity")

	// Unit revolute velocity about world z through the base origin:
	// v = z x p_tool = (0,0,1) x (0,0.35,0.4) = (-0.35, 0, 0).
	lin, ang, err = c.ToolVelocity(pos, []float64{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	vecAlmost(t, lin, [3]float64{-0.35, 0, 0}, 1e-12, "revolute linear velocity")
	vecAlmost(t, ang, [3]float64{0, 0, 1}, 1e-12, "revolute angular velocity")
}

func TestJacobianNumericCheck(t *testing.T) {
	c := mixedChain()
	check, err := c.CheckJacobian([]float64{0.3, 0.25})
	if err != nil {
		t.Fatal(err)
	}
	for _, ce := range check {
		if !ce.Within {
			t.Errorf("joint %d column error %g exceeds tolerance", ce.Joint, ce.MaxAbs)
		}
		if ce.OneSided != "" {
			t.Errorf("joint %d unexpectedly one-sided", ce.Joint)
		}
	}
}

func TestJacobianNumericCheckAtLimit(t *testing.T) {
	c := mixedChain()
	// Prismatic joint exactly at its lower limit: only a forward difference
	// is legal; no out-of-limit pose may be evaluated.
	check, err := c.CheckJacobian([]float64{0.3, 0})
	if err != nil {
		t.Fatal(err)
	}
	if check[1].OneSided != "forward" {
		t.Errorf("expected forward one-sided difference at lower limit, got %q", check[1].OneSided)
	}
	if !check[1].Within {
		t.Errorf("one-sided column error %g exceeds tolerance", check[1].MaxAbs)
	}
	// Upper limit -> backward difference.
	check, err = c.CheckJacobian([]float64{0.3, 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if check[1].OneSided != "backward" {
		t.Errorf("expected backward one-sided difference at upper limit, got %q", check[1].OneSided)
	}
}

func TestChainValidation(t *testing.T) {
	c := mixedChain()
	if err := c.Validate(); err != nil {
		t.Fatalf("valid chain rejected: %v", err)
	}
	c.Joints = nil
	if err := c.Validate(); err == nil {
		t.Error("expected empty chain to be rejected")
	}
	c = mixedChain()
	c.Joints = append(c.Joints, make([]Joint, 6)...)
	if err := c.Validate(); err == nil {
		t.Error("expected 8-joint chain to be rejected")
	}
	c = mixedChain()
	c.Joints[0].Type = "helical"
	if err := c.Validate(); err == nil {
		t.Error("expected unknown joint type to be rejected")
	}
}
