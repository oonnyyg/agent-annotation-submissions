package main

import (
	"fmt"
	"math"
)

// JointType distinguishes revolute and prismatic joints.
type JointType string

const (
	Revolute  JointType = "revolute"
	Prismatic JointType = "prismatic"
)

// Joint describes one standard-DH joint. The variable is theta for
// revolute joints and d for prismatic joints; the other fields are fixed.
type Joint struct {
	Name   string    `json:"name"`
	Type   JointType `json:"type"`
	Theta  float64   `json:"theta"`  // rad, revolute offset / fixed angle
	D      float64   `json:"d"`      // m, prismatic offset / fixed distance
	A      float64   `json:"a"`      // m, translation along x
	Alpha  float64   `json:"alpha"`  // rad, rotation about x
	Offset float64   `json:"offset"` // zero-position offset added to the variable
	Min    float64   `json:"min"`    // travel limit on variable+offset
	Max    float64   `json:"max"`
}

// Chain is a serial arm of 1..6 joints with fixed base and tool transforms.
type Chain struct {
	Name   string  `json:"name"`
	Joints []Joint `json:"joints"`
	Base   Mat4    `json:"base"` // fixed base transform (world -> joint0 parent)
	Tool   Mat4    `json:"tool"` // fixed tool transform (last joint frame -> tool)
}

// Sample is one joint-space query: positions and velocities per joint.
type Sample struct {
	Name       string    `json:"name"`
	Positions  []float64 `json:"positions"`  // joint variables (rad or m)
	Velocities []float64 `json:"velocities"` // joint rates (rad/s or m/s)
}

// Mat4 is a row-major 4x4 homogeneous transform.
type Mat4 [4][4]float64

// Identity returns the identity transform.
func Identity() Mat4 {
	var m Mat4
	for i := 0; i < 4; i++ {
		m[i][i] = 1
	}
	return m
}

// Mul returns a*b (apply b first, then a).
func (a Mat4) Mul(b Mat4) Mat4 {
	var c Mat4
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			for k := 0; k < 4; k++ {
				c[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return c
}

// Rot returns the 3x3 rotation block.
func (m Mat4) Rot() [3][3]float64 {
	var r [3][3]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i][j] = m[i][j]
		}
	}
	return r
}

// Trans returns the translation column.
func (m Mat4) Trans() [3]float64 {
	return [3]float64{m[0][3], m[1][3], m[2][3]}
}

// dhTransform applies the standard DH sequence: Rz(theta) Tz(d) Tx(a) Rx(alpha).
func dhTransform(theta, d, a, alpha float64) Mat4 {
	ct, st := math.Cos(theta), math.Sin(theta)
	ca, sa := math.Cos(alpha), math.Sin(alpha)
	return Mat4{
		{ct, -st * ca, st * sa, a * ct},
		{st, ct * ca, -ct * sa, a * st},
		{0, sa, ca, d},
		{0, 0, 0, 1},
	}
}

// validateTransform checks that m is a rigid transform: orthonormal rotation
// with determinant +1 and a proper homogeneous bottom row.
func validateTransform(name string, m Mat4) error {
	const tol = 1e-9
	if m[3] != [4]float64{0, 0, 0, 1} {
		return fmt.Errorf("%s: bottom row must be [0 0 0 1], got %v", name, m[3])
	}
	r := m.Rot()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			var dot float64
			for k := 0; k < 3; k++ {
				dot += r[k][i] * r[k][j]
			}
			want := 0.0
			if i == j {
				want = 1.0
			}
			if math.Abs(dot-want) > tol {
				return fmt.Errorf("%s: rotation not orthonormal (R^T R)[%d][%d]=%g, want %g", name, i, j, dot, want)
			}
		}
	}
	det := r[0][0]*(r[1][1]*r[2][2]-r[1][2]*r[2][1]) -
		r[0][1]*(r[1][0]*r[2][2]-r[1][2]*r[2][0]) +
		r[0][2]*(r[1][0]*r[2][1]-r[1][1]*r[2][0])
	if math.Abs(det-1) > tol {
		return fmt.Errorf("%s: rotation determinant %g, want +1", name, det)
	}
	return nil
}

// Validate checks chain structure, joint limits, and fixed transforms.
func (c *Chain) Validate() error {
	if len(c.Joints) < 1 || len(c.Joints) > 6 {
		return fmt.Errorf("chain must have 1..6 joints, got %d", len(c.Joints))
	}
	for i, j := range c.Joints {
		if j.Type != Revolute && j.Type != Prismatic {
			return fmt.Errorf("joint %d (%q): type must be %q or %q", i, j.Name, Revolute, Prismatic)
		}
		if !(j.Min <= j.Max) {
			return fmt.Errorf("joint %d (%q): min %g exceeds max %g", i, j.Name, j.Min, j.Max)
		}
	}
	if err := validateTransform("base", c.Base); err != nil {
		return err
	}
	if err := validateTransform("tool", c.Tool); err != nil {
		return err
	}
	return nil
}

// jointValue returns the DH variable (theta or d) for a joint given the
// sample position, enforcing travel limits on variable+offset.
func (c *Chain) jointValue(i int, pos float64) (float64, error) {
	j := c.Joints[i]
	v := pos + j.Offset
	if v < j.Min-1e-12 || v > j.Max+1e-12 {
		return 0, fmt.Errorf("joint %d (%q): position %g (with offset %g) out of limits [%g, %g]",
			i, j.Name, pos, j.Offset, j.Min, j.Max)
	}
	return v, nil
}

// jointTransform returns the DH transform for joint i at sample position pos.
func (c *Chain) jointTransform(i int, pos float64) (Mat4, error) {
	j := c.Joints[i]
	v, err := c.jointValue(i, pos)
	if err != nil {
		return Mat4{}, err
	}
	theta, d := j.Theta, j.D
	if j.Type == Revolute {
		theta += v
	} else {
		d += v
	}
	return dhTransform(theta, d, j.A, j.Alpha), nil
}

// ForwardKinematics computes the world-frame transform of every joint frame
// (index i is the frame after joint i) plus the tool frame as the last entry.
// All returned frames include the fixed base transform.
func (c *Chain) ForwardKinematics(positions []float64) ([]Mat4, error) {
	if len(positions) != len(c.Joints) {
		return nil, fmt.Errorf("expected %d positions, got %d", len(c.Joints), len(positions))
	}
	frames := make([]Mat4, 0, len(c.Joints)+1)
	t := c.Base
	for i := range c.Joints {
		jt, err := c.jointTransform(i, positions[i])
		if err != nil {
			return nil, err
		}
		t = t.Mul(jt)
		frames = append(frames, t)
	}
	// Tool offset rides on the last joint frame; it is not added in world axes.
	t = t.Mul(c.Tool)
	frames = append(frames, t)
	return frames, nil
}

// ToolPose returns the world-frame tool transform for the given positions.
func (c *Chain) ToolPose(positions []float64) (Mat4, error) {
	frames, err := c.ForwardKinematics(positions)
	if err != nil {
		return Mat4{}, err
	}
	return frames[len(frames)-1], nil
}
