package main

// ForwardKinematics computes, in world coordinates, the frame of every joint
// (after applying its DH transform) and the final tool pose. The tool offset
// is composed as a rigid transform, so the end-effector rotation affects the
// tool origin position.
func (c *Chain) ForwardKinematics(q []float64) (jointFrames []Transform, tool Transform, err error) {
	if err := c.checkLimits(q); err != nil {
		return nil, Transform{}, err
	}
	t := Transform{R: c.Base.Rotation, P: c.Base.Translation}
	jointFrames = make([]Transform, len(c.Joints))
	for i, j := range c.Joints {
		t = t.Mul(j.jointTransform(q[i]))
		jointFrames[i] = t
	}
	tool = t.Mul(Transform{R: c.Tool.Rotation, P: c.Tool.Translation})
	return jointFrames, tool, nil
}

// Jacobian is the 6xN geometric Jacobian of the tool origin expressed in the
// world frame: rows 0-2 linear velocity, rows 3-5 angular velocity.
type Jacobian struct {
	Cols [][]float64 // each column has 6 entries
}

// GeometricJacobian builds the world-frame Jacobian at configuration q.
// For a revolute joint i with world z-axis z and origin o:
//
//	Jv = z x (p_tool - o),  Jw = z
//
// For a prismatic joint: Jv = z, Jw = 0. A fixed tool offset changes
// p_tool and therefore the linear-velocity rows.
func (c *Chain) GeometricJacobian(q []float64) (*Jacobian, Transform, error) {
	frames, tool, err := c.ForwardKinematics(q)
	if err != nil {
		return nil, Transform{}, err
	}
	jac := &Jacobian{Cols: make([][]float64, len(c.Joints))}
	// Frame before joint i: base for i=0, else frames[i-1].
	prev := Transform{R: c.Base.Rotation, P: c.Base.Translation}
	for i, j := range c.Joints {
		z := Vec3{prev.R[0][2], prev.R[1][2], prev.R[2][2]}
		o := prev.P
		col := make([]float64, 6)
		if j.Type == JointRevolute {
			jv := z.Cross(tool.P.Sub(o))
			col[0], col[1], col[2] = jv[0], jv[1], jv[2]
			col[3], col[4], col[5] = z[0], z[1], z[2]
		} else {
			col[0], col[1], col[2] = z[0], z[1], z[2]
		}
		jac.Cols[i] = col
		prev = frames[i]
	}
	return jac, tool, nil
}

// Twist is the tool velocity in world coordinates: linear then angular.
type Twist struct {
	Linear  Vec3
	Angular Vec3
}

// Apply multiplies the Jacobian by joint velocities qd.
func (j *Jacobian) Apply(qd []float64) Twist {
	var t Twist
	for i, col := range j.Cols {
		for r := 0; r < 3; r++ {
			t.Linear[r] += col[r] * qd[i]
			t.Angular[r] += col[r+3] * qd[i]
		}
	}
	return t
}
