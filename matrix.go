package main

import (
	"errors"
	"math"
)

// Vec3 is a 3D vector in metres or radians/second depending on context.
type Vec3 [3]float64

// Mat3 is a row-major 3x3 matrix.
type Mat3 [3][3]float64

// Transform is a rigid-body transform: rotation R and translation P.
type Transform struct {
	R Mat3
	P Vec3
}

func IdentityTransform() Transform {
	return Transform{R: Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
}

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a Vec3) Scale(s float64) Vec3 { return Vec3{a[0] * s, a[1] * s, a[2] * s} }

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func (a Vec3) Norm() float64 { return math.Sqrt(a[0]*a[0] + a[1]*a[1] + a[2]*a[2]) }

func (m Mat3) MulVec(v Vec3) Vec3 {
	return Vec3{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

func (m Mat3) MulMat(n Mat3) Mat3 {
	var r Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i][j] = m[i][0]*n[0][j] + m[i][1]*n[1][j] + m[i][2]*n[2][j]
		}
	}
	return r
}

func (m Mat3) Transpose() Mat3 {
	var r Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i][j] = m[j][i]
		}
	}
	return r
}

func (m Mat3) Det() float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// ValidateRotation checks orthonormality and determinant == +1.
func ValidateRotation(m Mat3, tol float64) error {
	ident := Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	prod := m.MulMat(m.Transpose())
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if math.Abs(prod[i][j]-ident[i][j]) > tol {
				return errors.New("rotation matrix is not orthonormal")
			}
		}
	}
	if d := m.Det(); math.Abs(d-1) > tol {
		return errors.New("rotation matrix determinant is not +1")
	}
	return nil
}

// Mul composes transforms: result = a * b (apply b first, then a).
func (a Transform) Mul(b Transform) Transform {
	return Transform{
		R: a.R.MulMat(b.R),
		P: a.R.MulVec(b.P).Add(a.P),
	}
}

func RotZ(theta float64) Transform {
	c, s := math.Cos(theta), math.Sin(theta)
	return Transform{R: Mat3{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}}
}

func RotX(alpha float64) Transform {
	c, s := math.Cos(alpha), math.Sin(alpha)
	return Transform{R: Mat3{{1, 0, 0}, {0, c, -s}, {0, s, c}}}
}

func TransZ(d float64) Transform {
	return Transform{R: Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, P: Vec3{0, 0, d}}
}

func TransX(a float64) Transform {
	return Transform{R: Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, P: Vec3{a, 0, 0}}
}
