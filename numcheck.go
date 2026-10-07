package main

import "math"

// ColumnCheck reports the numerical-difference error for one Jacobian column.
type ColumnCheck struct {
	Joint    string     `json:"joint"`
	Scheme   string     `json:"scheme"` // "central", "forward", or "backward"
	Step     float64    `json:"step"`
	Numeric  Vec3       `json:"numericColumn"` // d(position)/dq from finite differences
	Analytic [3]float64 `json:"analyticColumn"`
	MaxError float64    `json:"maxError"`
}

// NumericalPositionJacobian verifies the linear-velocity rows of the
// geometric Jacobian by finite differences of the tool origin position.
// Samples sitting on a joint limit use a one-sided difference that stays
// inside the legal range, so no out-of-limit pose is ever evaluated.
func (c *Chain) NumericalPositionJacobian(q []float64, jac *Jacobian) ([]ColumnCheck, error) {
	const (
		h      = 1e-6
		epsLim = 1e-9
	)
	_, tool0, err := c.ForwardKinematics(q)
	if err != nil {
		return nil, err
	}
	checks := make([]ColumnCheck, len(c.Joints))
	for i, j := range c.Joints {
		chk := ColumnCheck{Joint: j.Name}
		for r := 0; r < 3; r++ {
			chk.Analytic[r] = jac.Cols[i][r]
		}
		qUp, qDown := q[i]+h, q[i]-h
		canUp := qUp <= j.Limit[1]+epsLim
		canDown := qDown >= j.Limit[0]-epsLim
		perturb := func(delta float64) (Vec3, error) {
			qq := make([]float64, len(q))
			copy(qq, q)
			qq[i] += delta
			_, t, err := c.ForwardKinematics(qq)
			return t.P, err
		}
		var num Vec3
		switch {
		case canUp && canDown:
			up, err := perturb(h)
			if err != nil {
				return nil, err
			}
			down, err := perturb(-h)
			if err != nil {
				return nil, err
			}
			num = up.Sub(down).Scale(1 / (2 * h))
			chk.Scheme, chk.Step = "central", h
		case canUp:
			up, err := perturb(h)
			if err != nil {
				return nil, err
			}
			num = up.Sub(tool0.P).Scale(1 / h)
			chk.Scheme, chk.Step = "forward", h
		case canDown:
			down, err := perturb(-h)
			if err != nil {
				return nil, err
			}
			num = tool0.P.Sub(down).Scale(1 / h)
			chk.Scheme, chk.Step = "backward", h
		default:
			num = Vec3{}
			chk.Scheme, chk.Step = "degenerate", 0
		}
		chk.Numeric = num
		for r := 0; r < 3; r++ {
			chk.MaxError = math.Max(chk.MaxError, math.Abs(num[r]-chk.Analytic[r]))
		}
		checks[i] = chk
	}
	return checks, nil
}
