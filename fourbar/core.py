"""Core kinematics for a planar four-bar linkage.

Layout (all angles in radians, lengths in arbitrary but consistent units):

    A = (0, 0)      fixed pivot, ground start
    D = (r1, 0)     fixed pivot, ground end
    B = A + r2 * (cos(theta), sin(theta))   input moving pivot
    C               output moving pivot, intersection of
                    circle(B, r3) and circle(D, r4)

The assembly branch is selected by the sign of the 2D cross product
cross(D - B, C - B).  The sign is chosen once (via the ``branch``
parameter, +1 or -1) and kept for the whole angle sweep.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

# Relative tolerance used to detect tangent (singular) configurations.
SINGULAR_TOL = 1e-9

STATUS_OK = "ok"
STATUS_SINGULAR = "singular"
STATUS_UNREACHABLE = "unreachable"
STATUS_COINCIDENT = "non_unique"


@dataclass(frozen=True)
class FourBar:
    """Link lengths.  r1 ground, r2 input, r3 coupler, r4 output."""

    r1: float
    r2: float
    r3: float
    r4: float

    def __post_init__(self) -> None:
        for name in ("r1", "r2", "r3", "r4"):
            value = getattr(self, name)
            if not (isinstance(value, (int, float)) and math.isfinite(value) and value > 0):
                raise ValueError(f"link length {name} must be a positive finite number, got {value!r}")


@dataclass
class ConfigResult:
    """Result for one input angle."""

    theta: float
    omega2: float
    status: str
    B: tuple[float, float] | None = None
    C: tuple[float, float] | None = None
    vB: tuple[float, float] | None = None
    vC: tuple[float, float] | None = None
    omega3: float | None = None
    omega4: float | None = None
    probe: tuple[float, float] | None = None
    probe_velocity: tuple[float, float] | None = None
    length_residuals: tuple[float, float] | None = None
    velocity_residuals: tuple[float, float] | None = None
    message: str = ""


def _cross(ax: float, ay: float, bx: float, by: float) -> float:
    return ax * by - ay * bx


def solve_configuration(
    link: FourBar,
    theta: float,
    omega2: float,
    branch: int,
    probe: tuple[float, float] | None = None,
) -> ConfigResult:
    """Solve position and velocity for one input angle.

    ``branch`` is +1 or -1 and fixes the sign of cross(D - B, C - B).
    ``probe`` is an optional (u, v) point in coupler-local coordinates
    (origin at B, x axis along B->C, y axis rotated +90 degrees).
    """
    if branch not in (1, -1):
        raise ValueError("branch must be +1 or -1")

    bx = link.r2 * math.cos(theta)
    by = link.r2 * math.sin(theta)
    dx, dy = link.r1, 0.0
    result = ConfigResult(theta=theta, omega2=omega2, status=STATUS_OK, B=(bx, by))

    # Vector from B to the right fixed pivot D.
    ex = dx - bx
    ey = dy - by
    d = math.hypot(ex, ey)
    r3, r4 = link.r3, link.r4

    if d == 0.0:
        if r3 == r4:
            result.status = STATUS_COINCIDENT
            result.message = "circles coincide: configuration is not unique"
        else:
            result.status = STATUS_UNREACHABLE
            result.message = "B coincides with D but r3 != r4: no intersection"
        return result

    # Distance along B->D of the radical line foot, and height squared.
    a = (d * d + r3 * r3 - r4 * r4) / (2.0 * d)
    h_sq = r3 * r3 - a * a

    scale = max(r3, r4, d)
    tol = SINGULAR_TOL * scale * scale

    if h_sq < -tol:
        result.status = STATUS_UNREACHABLE
        result.message = "circles are separate or nested: no intersection"
        return result

    singular = h_sq <= tol
    h = math.sqrt(h_sq) if h_sq > 0.0 else 0.0

    ux, uy = ex / d, ey / d  # unit vector B -> D
    # Foot of the intersection chord.
    fx = bx + a * ux
    fy = by + a * uy
    # Perpendicular (rotated +90 deg).  Branch sign picks the side.
    px, py = -uy, ux
    cx = fx + branch * h * px
    cy = fy + branch * h * py
    result.C = (cx, cy)

    if singular:
        result.status = STATUS_SINGULAR
        result.message = "circles tangent: unique configuration, velocity undetermined"
        _fill_position_only(result, link, probe)
        return result

    # Velocity solve.  B moves with the input crank.
    vbx = -link.r2 * omega2 * math.sin(theta)
    vby = link.r2 * omega2 * math.cos(theta)
    result.vB = (vbx, vby)

    # Constraints:
    #   (C - B) . (vC - vB) = 0
    #   (C - D) . vC        = 0
    cbx, cby = cx - bx, cy - by
    cdx, cdy = cx - dx, cy - dy
    rhs1 = cbx * vbx + cby * vby
    det = _cross(cbx, cby, cdx, cdy)
    # det is nonzero away from tangency (guaranteed by the singular check).
    # Solve the 2x2 system directly:
    #   cbx*vcx + cby*vcy = rhs1
    #   cdx*vcx + cdy*vcy = 0
    vcx = (rhs1 * cdy) / det
    vcy = (-rhs1 * cdx) / det
    result.vC = (vcx, vcy)

    # Angular velocities of coupler (omega3) and output (omega4):
    # v_rel = omega x r  =>  omega = cross(r, v_rel) / |r|^2.
    result.omega3 = _cross(cbx, cby, vcx - vbx, vcy - vby) / (r3 * r3)
    result.omega4 = _cross(cdx, cdy, vcx, vcy) / (r4 * r4)

    _fill_probe(result, link, probe)
    _fill_residuals(result, link)
    return result


def _fill_position_only(result: ConfigResult, link: FourBar, probe: tuple[float, float] | None) -> None:
    """Singular case: keep positions, leave velocities as None."""
    if probe is not None and result.C is not None and result.B is not None:
        bx, by = result.B
        cx, cy = result.C
        e1x, e1y = (cx - bx) / link.r3, (cy - by) / link.r3
        e2x, e2y = -e1y, e1x
        u, v = probe
        result.probe = (bx + u * e1x + v * e2x, by + u * e1y + v * e2y)
    result.length_residuals = _length_residuals(result, link)


def _fill_probe(result: ConfigResult, link: FourBar, probe: tuple[float, float] | None) -> None:
    if probe is None or result.C is None or result.B is None:
        return
    bx, by = result.B
    cx, cy = result.C
    e1x, e1y = (cx - bx) / link.r3, (cy - by) / link.r3
    e2x, e2y = -e1y, e1x
    u, v = probe
    px = bx + u * e1x + v * e2x
    py = by + u * e1y + v * e2y
    result.probe = (px, py)
    if result.vB is not None and result.omega3 is not None:
        vbx, vby = result.vB
        w = result.omega3
        result.probe_velocity = (vbx - w * (py - by), vby + w * (px - bx))


def _length_residuals(result: ConfigResult, link: FourBar) -> tuple[float, float]:
    bx, by = result.B
    cx, cy = result.C
    r_coupler = math.hypot(cx - bx, cy - by) - link.r3
    r_output = math.hypot(cx - link.r1, cy) - link.r4
    return (r_coupler, r_output)


def _fill_residuals(result: ConfigResult, link: FourBar) -> None:
    result.length_residuals = _length_residuals(result, link)
    bx, by = result.B
    cx, cy = result.C
    vbx, vby = result.vB
    vcx, vcy = result.vC
    res1 = (cx - bx) * (vcx - vbx) + (cy - by) * (vcy - vby)
    res2 = (cx - link.r1) * vcx + cy * vcy
    result.velocity_residuals = (res1, res2)
