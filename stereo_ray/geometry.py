"""Core geometry: radial undistortion, ray triangulation, reprojection.

Conventions
-----------
- Pixels are in image coordinates, metric quantities in metres.
- Each camera gives positive focal lengths ``fx``, ``fy``, principal point
  ``(cx, cy)``, optical centre ``center`` in world coordinates, and a
  right-handed orthonormal rotation ``rotation`` mapping camera-frame
  vectors to the world frame. Positive depth is along the local third axis.
- Radial distortion (two coefficients, no tangential part) magnifies the
  normalised radius by ``1 + k1 * r**2 + k2 * r**4``.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field


class ReconstructionError(ValueError):
    """Raised when a single point cannot be reconstructed geometrically."""


@dataclass(frozen=True)
class Camera:
    fx: float
    fy: float
    cx: float
    cy: float
    center: tuple[float, float, float]
    rotation: tuple[tuple[float, float, float], ...]  # camera -> world, 3x3
    k1: float = 0.0
    k2: float = 0.0

    def __post_init__(self) -> None:
        if self.fx <= 0 or self.fy <= 0:
            raise ValueError("focal lengths must be positive")
        if self.k1 < 0 or self.k2 < 0:
            raise ValueError("distortion coefficients must be non-negative")
        if len(self.rotation) != 3 or any(len(r) != 3 for r in self.rotation):
            raise ValueError("rotation must be 3x3")
        _check_rotation(self.rotation)


def _check_rotation(rot: tuple[tuple[float, float, float], ...]) -> None:
    cols = [[rot[r][c] for r in range(3)] for c in range(3)]
    for i in range(3):
        norm = math.sqrt(sum(v * v for v in cols[i]))
        if abs(norm - 1.0) > 1e-6:
            raise ValueError("rotation columns must be unit length")
        for j in range(i + 1, 3):
            dot = sum(a * b for a, b in zip(cols[i], cols[j]))
            if abs(dot) > 1e-6:
                raise ValueError("rotation columns must be orthogonal")
    det = (
        rot[0][0] * (rot[1][1] * rot[2][2] - rot[1][2] * rot[2][1])
        - rot[0][1] * (rot[1][0] * rot[2][2] - rot[1][2] * rot[2][0])
        + rot[0][2] * (rot[1][0] * rot[2][1] - rot[1][1] * rot[2][0])
    )
    if det <= 0.0:
        raise ValueError("rotation must be right-handed (det = +1)")


def _mat_vec(m, v):
    return tuple(sum(m[r][c] * v[c] for c in range(3)) for r in range(3))


def _mat_t_vec(m, v):
    return tuple(sum(m[c][r] * v[c] for c in range(3)) for r in range(3))


def undistort_normalized(xd: float, yd: float, k1: float, k2: float) -> tuple[float, float]:
    """Invert the radial model: distorted = undistorted * (1 + k1 r^2 + k2 r^4).

    Newton iteration on the undistorted radius; the magnification factor is
    strictly increasing for non-negative coefficients, so the root is unique.
    """
    rd = math.hypot(xd, yd)
    if rd == 0.0 or (k1 == 0.0 and k2 == 0.0):
        return xd, yd
    r = rd  # initial guess: undistorted radius near distorted radius
    for _ in range(50):
        r2 = r * r
        f = r * (1.0 + k1 * r2 + k2 * r2 * r2) - rd
        df = 1.0 + 3.0 * k1 * r2 + 5.0 * k2 * r2 * r2
        step = f / df
        r -= step
        if abs(step) < 1e-15 * max(1.0, r):
            break
    scale = r / rd
    return xd * scale, yd * scale


def pixel_to_world_ray(cam: Camera, u: float, v: float):
    """Return (origin, unit_direction) of the undistorted ray in world frame."""
    xd = (u - cam.cx) / cam.fx
    yd = (v - cam.cy) / cam.fy
    xn, yn = undistort_normalized(xd, yd, cam.k1, cam.k2)
    d_cam = (xn, yn, 1.0)
    norm = math.sqrt(sum(c * c for c in d_cam))
    d_cam = tuple(c / norm for c in d_cam)
    return cam.center, _mat_vec(cam.rotation, d_cam)


def project_point(cam: Camera, point) -> tuple[float, float, float]:
    """Project a world point; returns (u, v, depth). Fails on non-positive depth."""
    rel = tuple(p - c for p, c in zip(point, cam.center))
    pc = _mat_t_vec(cam.rotation, rel)  # world -> camera is R^T
    depth = pc[2]
    if depth <= 0.0:
        raise ReconstructionError("estimated point has non-positive depth in camera")
    xn, yn = pc[0] / depth, pc[1] / depth
    r2 = xn * xn + yn * yn
    mag = 1.0 + cam.k1 * r2 + cam.k2 * r2 * r2
    u = cam.fx * xn * mag + cam.cx
    v = cam.fy * yn * mag + cam.cy
    return u, v, depth


def reconstruct_point(
    cam1: Camera,
    cam2: Camera,
    px1: tuple[float, float],
    px2: tuple[float, float],
    min_angle_deg: float = 1.0,
) -> dict:
    """Triangulate one matched pair via closest points of the two world rays.

    Returns a dict with the midpoint estimate, the closest distance between
    the skew rays, and per-camera reprojection residuals in pixels.
    Raises ReconstructionError for degenerate or behind-camera geometry.
    """
    if not (0.0 < min_angle_deg < 90.0):
        raise ValueError("angle threshold must be in (0, 90) degrees")

    c1, d1 = pixel_to_world_ray(cam1, *px1)
    c2, d2 = pixel_to_world_ray(cam2, *px2)

    baseline = math.dist(c1, c2)
    if baseline == 0.0:
        raise ReconstructionError("optical centres coincide")

    cos_angle = sum(a * b for a, b in zip(d1, d2))
    # Angle between the *lines* (parallel and anti-parallel are both bad).
    line_angle = math.degrees(math.acos(min(1.0, abs(cos_angle))))
    if line_angle < min_angle_deg:
        raise ReconstructionError(
            f"sight lines nearly parallel: {line_angle:.6f} deg < {min_angle_deg} deg"
        )

    # Closest points: solve for s, t minimising |(c1 + s d1) - (c2 + t d2)|.
    w = tuple(a - b for a, b in zip(c1, c2))
    b = cos_angle
    d = sum(a * b_ for a, b_ in zip(d1, w))
    e = sum(a * b_ for a, b_ in zip(d2, w))
    denom = 1.0 - b * b
    s = (b * e - d) / denom
    t = (e - b * d) / denom

    p1 = tuple(c + s * d for c, d in zip(c1, d1))
    p2 = tuple(c + t * d for c, d in zip(c2, d2))
    distance = math.dist(p1, p2)
    midpoint = tuple((a + b_) / 2.0 for a, b_ in zip(p1, p2))

    if s <= 0.0 or t <= 0.0:
        raise ReconstructionError(
            "closest point lies behind a camera centre (no valid forward intersection)"
        )

    u1, v1, depth1 = project_point(cam1, midpoint)
    u2, v2, depth2 = project_point(cam2, midpoint)

    return {
        "point": list(midpoint),
        "ray_distance": distance,
        "ray_angle_deg": line_angle,
        "depths": [depth1, depth2],
        "residuals_px": [
            [u1 - px1[0], v1 - px1[1]],
            [u2 - px2[0], v2 - px2[1]],
        ],
    }
