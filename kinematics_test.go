package main

import (
	"math"
	"testing"
)

func identPose() PoseJSON {
	return PoseJSON{Rotation: Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
}

func planar2R() *Chain {
	return &Chain{
		Joints: []Joint{
			{Name: "j1", Type: JointRevolute, A: 1, Limit: [2]float64{-math.Pi, math.Pi}},
			{Name: "j2", Type: JointRevolute, A: 1, Limit: [2]float64{-math.Pi, math.Pi}},
		},
		Base: identPose(),
		Tool: identPose(),
	}
}

func vecNear(t *testing.T, got Vec3, want Vec3, tol float64, msg string) {
	t.Helper()
	for i := 0; i < 3; i++ {
		if math.Abs(got[i]-want[i]) > tol {
			t.Fatalf("%s: got %v, want %v", msg, got, want)
		}
	}
}

func TestPlanar2RForward(t *testing.T) {
	c := planar2R()
	frames, tool, err := c.ForwardKinematics([]float64{math.Pi / 2, 0})
	if err != nil {
		t.Fatal(err)
	}
	vecNear(t, frames[0].P, Vec3{0, 1, 0}, 1e-12, "joint1 origin")
	vecNear(t, tool.P, Vec3{0, 2, 0}, 1e-12, "tool origin")
}

func TestPlanar2RJacobianAndTwist(t *testing.T) {
	c := planar2R()
	jac, _, err := c.GeometricJacobian([]float64{math.Pi / 2, 0})
	if err != nil {
		t.Fatal(err)
	}
	vecNear(t, Vec3{jac.Cols[0][0], jac.Cols[0][1], jac.Cols[0][2]}, Vec3{-2, 0, 0}, 1e-12, "Jv col1")
	vecNear(t, Vec3{jac.Cols[1][0], jac.Cols[1][1], jac.Cols[1][2]}, Vec3{-1, 0, 0}, 1e-12, "Jv col2")
	vecNear(t, Vec3{jac.Cols[0][3], jac.Cols[0][4], jac.Cols[0][5]}, Vec3{0, 0, 1}, 1e-12, "Jw col1")
	tw := jac.Apply([]float64{1, 1})
	vecNear(t, tw.Linear, Vec3{-3, 0, 0}, 1e-12, "linear velocity")
	vecNear(t, tw.Angular, Vec3{0, 0, 2}, 1e-12, "angular velocity")
}

func TestMixedChainWithPrismatic(t *testing.T) {
	c := &Chain{
		Joints: []Joint{
			{Name: "lift", Type: JointPrismatic, Limit: [2]float64{0, 0.5}},
			{Name: "wrist", Type: JointRevolute, A: 0.3, Limit: [2]float64{-math.Pi, math.Pi}},
		},
		Base: identPose(),
		Tool: identPose(),
	}
	_, tool, err := c.ForwardKinematics([]float64{0.2, math.Pi / 2})
	if err != nil {
		t.Fatal(err)
	}
	vecNear(t, tool.P, Vec3{0, 0.3, 0.2}, 1e-12, "mixed chain tool origin")
	jac, _, err := c.GeometricJacobian([]float64{0.2, math.Pi / 2})
	if err != nil {
		t.Fatal(err)
	}
	// Prismatic column: Jv = z = (0,0,1), Jw = 0.
	vecNear(t, Vec3{jac.Cols[0][0], jac.Cols[0][1], jac.Cols[0][2]}, Vec3{0, 0, 1}, 1e-12, "prismatic Jv")
	vecNear(t, Vec3{jac.Cols[0][3], jac.Cols[0][4], jac.Cols[0][5]}, Vec3{0, 0, 0}, 1e-12, "prismatic Jw")
	tw := jac.Apply([]float64{0.1, 2})
	vecNear(t, tw.Linear, Vec3{-0.6, 0, 0.1}, 1e-12, "mixed twist linear")
	vecNear(t, tw.Angular, Vec3{0, 0, 2}, 1e-12, "mixed twist angular")
}
