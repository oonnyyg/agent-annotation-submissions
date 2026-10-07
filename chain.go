package main

import (
	"encoding/json"
	"fmt"
)

const (
	JointRevolute  = "revolute"
	JointPrismatic = "prismatic"
)

const rotationTolerance = 1e-6

// Joint describes one standard-DH joint. The joint transform is
// RotZ(theta) * TransZ(d) * TransX(a) * RotX(alpha), where exactly one of
// theta (revolute) or d (prismatic) varies with the joint variable q.
type Joint struct {
	Name   string     `json:"name"`
	Type   string     `json:"type"` // "revolute" or "prismatic"
	Theta  float64    `json:"theta"`
	D      float64    `json:"d"`
	A      float64    `json:"a"`
	Alpha  float64    `json:"alpha"`
	Offset float64    `json:"offset"` // zero-position offset added to q
	Limit  [2]float64 `json:"limit"`  // [min, max] for q, metres or radians
}

// PoseJSON is a fixed rigid transform from JSON.
type PoseJSON struct {
	Rotation    Mat3 `json:"rotation"`
	Translation Vec3 `json:"translation"`
}

// Chain is the full robot description.
type Chain struct {
	Joints []Joint  `json:"joints"`
	Base   PoseJSON `json:"base"`
	Tool   PoseJSON `json:"tool"`
}

// Sample is one joint configuration with joint velocities.
type Sample struct {
	Name string    `json:"name"`
	Q    []float64 `json:"q"`
	Qd   []float64 `json:"qd"`
}

// SamplesFile wraps the sample list for JSON decoding.
type SamplesFile struct {
	Samples []Sample `json:"samples"`
}

func (c *Chain) Validate() error {
	n := len(c.Joints)
	if n < 1 || n > 6 {
		return fmt.Errorf("chain must have 1 to 6 joints, got %d", n)
	}
	for i, j := range c.Joints {
		if j.Type != JointRevolute && j.Type != JointPrismatic {
			return fmt.Errorf("joint %d (%q): type must be %q or %q", i, j.Name, JointRevolute, JointPrismatic)
		}
		if j.Limit[0] > j.Limit[1] {
			return fmt.Errorf("joint %d (%q): limit min %v exceeds max %v", i, j.Name, j.Limit[0], j.Limit[1])
		}
	}
	if err := ValidateRotation(c.Base.Rotation, rotationTolerance); err != nil {
		return fmt.Errorf("base rotation invalid: %w", err)
	}
	if err := ValidateRotation(c.Tool.Rotation, rotationTolerance); err != nil {
		return fmt.Errorf("tool rotation invalid: %w", err)
	}
	return nil
}

// checkLimits returns an error if any component of q violates joint limits.
func (c *Chain) checkLimits(q []float64) error {
	if len(q) != len(c.Joints) {
		return fmt.Errorf("expected %d joint values, got %d", len(c.Joints), len(q))
	}
	for i, j := range c.Joints {
		if q[i] < j.Limit[0] || q[i] > j.Limit[1] {
			return fmt.Errorf("joint %d (%q): value %v outside limit [%v, %v]",
				i, j.Name, q[i], j.Limit[0], j.Limit[1])
		}
	}
	return nil
}

// jointTransform computes the DH transform of joint i at joint value q.
func (j Joint) jointTransform(q float64) Transform {
	theta, d := j.Theta, j.D
	if j.Type == JointRevolute {
		theta += q + j.Offset
	} else {
		d += q + j.Offset
	}
	return RotZ(theta).Mul(TransZ(d)).Mul(TransX(j.A)).Mul(RotX(j.Alpha))
}

func decodeChain(data []byte) (*Chain, error) {
	var c Chain
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing chain JSON: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
