package main

import (
	"fmt"
	"math"
)

// Jacobian is the 6xN geometric Jacobian of the tool origin, expressed in
// the world frame: rows 0-2 are linear velocity, rows 3-5 angular velocity.
type Jacobian [6][]float64

func sub(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

// GeometricJacobian computes the world-frame geometric Jacobian of the tool
// origin. Column i corresponds to joint i: for a revolute joint the linear
// part is z_i x (p_tool - p_i) and the angular part is z_i; for a prismatic
// joint the linear part is z_i and the angular part is zero. z_i and p_i are
// the axis and origin of joint i's parent frame (i.e. the frame before joint
// i's variable is applied), expressed in the world frame.
func (c *Chain) GeometricJacobian(positions []float64) (Jacobian, error) {
	frames, err := c.ForwardKinematics(positions)
	if err != nil {
		return Jacobian{}, err
	}
	pTool := frames[len(frames)-1].Trans()

	// Parent frames: base for joint 0, else the frame after the previous joint.
	parents := make([]Mat4, len(c.Joints))
	parents[0] = c.Base
	for i := 1; i < len(c.Joints); i++ {
		parents[i] = frames[i-1]
	}

	var jac Jacobian
	for r := 0; r < 6; r++ {
		jac[r] = make([]float64, len(c.Joints))
	}
	for i, j := range c.Joints {
		r := parents[i].Rot()
		z := [3]float64{r[0][2], r[1][2], r[2][2]}
		if j.Type == Revolute {
			lin := cross(z, sub(pTool, parents[i].Trans()))
			for k := 0; k < 3; k++ {
				jac[k][i] = lin[k]
				jac[k+3][i] = z[k]
			}
		} else {
			for k := 0; k < 3; k++ {
				jac[k][i] = z[k]
				jac[k+3][i] = 0
			}
		}
	}
	return jac, nil
}

// ToolVelocity multiplies the Jacobian by joint velocities, returning the
// tool-origin linear velocity (m/s) and angular velocity (rad/s) in the
// world frame. The fixed tool offset affects linear velocity through the
// cross-product term of revolute columns.
func (c *Chain) ToolVelocity(positions, velocities []float64) (lin, ang [3]float64, err error) {
	if len(velocities) != len(c.Joints) {
		return lin, ang, fmt.Errorf("expected %d velocities, got %d", len(c.Joints), len(velocities))
	}
	jac, err := c.GeometricJacobian(positions)
	if err != nil {
		return lin, ang, err
	}
	for i := range c.Joints {
		for k := 0; k < 3; k++ {
			lin[k] += jac[k][i] * velocities[i]
			ang[k] += jac[k+3][i] * velocities[i]
		}
	}
	return lin, ang, nil
}

// ColumnError reports the max-abs difference between an analytic Jacobian
// column and its finite-difference counterpart.
type ColumnError struct {
	Joint    int     `json:"joint"`
	Name     string  `json:"name"`
	MaxAbs   float64 `json:"max_abs_error"`
	Within   bool    `json:"within_tolerance"`
	OneSided string  `json:"one_sided,omitempty"` // "forward" or "backward" at limits
}

// CheckJacobian numerically differentiates the tool-origin position (rows
// 0-2 of the Jacobian) and reports per-column errors. Central differences
// are used in the interior; at travel limits a legal one-sided difference
// is used so no out-of-limit pose is ever evaluated.
func (c *Chain) CheckJacobian(positions []float64) ([]ColumnError, error) {
	const (
		h   = 1e-6
		tol = 1e-4
	)
	jac, err := c.GeometricJacobian(positions)
	if err != nil {
		return nil, err
	}
	p0, err := c.ToolPose(positions)
	if err != nil {
		return nil, err
	}
	base := p0.Trans()

	errs := make([]ColumnError, len(c.Joints))
	for i, j := range c.Joints {
		ce := ColumnError{Joint: i, Name: j.Name}
		// Offset is folded into the limit check, so compare against
		// limit-minus-offset in raw sample coordinates.
		lo, hi := j.Min-j.Offset, j.Max-j.Offset
		var num [3]float64
		switch {
		case positions[i]-h >= lo && positions[i]+h <= hi:
			pp, pm := append([]float64(nil), positions...), append([]float64(nil), positions...)
			pp[i] += h
			pm[i] -= h
			tp, err1 := c.ToolPose(pp)
			tm, err2 := c.ToolPose(pm)
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("joint %d: unexpected limit violation in central difference", i)
			}
			for k := 0; k < 3; k++ {
				num[k] = (tp.Trans()[k] - tm.Trans()[k]) / (2 * h)
			}
		case positions[i]+h <= hi:
			ce.OneSided = "forward"
			pp := append([]float64(nil), positions...)
			pp[i] += h
			tp, err1 := c.ToolPose(pp)
			if err1 != nil {
				return nil, fmt.Errorf("joint %d: unexpected limit violation in forward difference", i)
			}
			for k := 0; k < 3; k++ {
				num[k] = (tp.Trans()[k] - base[k]) / h
			}
		case positions[i]-h >= lo:
			ce.OneSided = "backward"
			pm := append([]float64(nil), positions...)
			pm[i] -= h
			tm, err1 := c.ToolPose(pm)
			if err1 != nil {
				return nil, fmt.Errorf("joint %d: unexpected limit violation in backward difference", i)
			}
			for k := 0; k < 3; k++ {
				num[k] = (base[k] - tm.Trans()[k]) / h
			}
		default:
			return nil, fmt.Errorf("joint %d (%q): travel range too small for finite difference step %g", i, j.Name, h)
		}
		for k := 0; k < 3; k++ {
			if d := math.Abs(num[k] - jac[k][i]); d > ce.MaxAbs {
				ce.MaxAbs = d
			}
		}
		ce.Within = ce.MaxAbs <= tol
		errs[i] = ce
	}
	return errs, nil
}
