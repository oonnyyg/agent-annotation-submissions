"""Core kinematics for a planar four-bar linkage (stdlib only, radians)."""

from __future__ import annotations

import math
from dataclasses import dataclass

OK = "ok"
SINGULAR = "singular"
UNREACHABLE = "unreachable"
AMBIGUOUS = "ambiguous"

SINGULAR_TOL = 1e-9


@dataclass
class Linkage:
    ground: float
    crank: float
    coupler: float
    rocker: float

    def __post_init__(self) -> None:
        for name in ("ground", "crank", "coupler", "rocker"):
            if not getattr(self, name) > 0.0:
                raise ValueError(f"link length must be positive: {name}")


@dataclass
class ConfigResult:
    theta: float
    omega_in: float
    status: str
    branch: int
    a: tuple[float, float] | None = None
    b: tuple[float, float] | None = None
    omega_coupler: float | None = None
    omega_rocker: float | None = None
    b_vel: tuple[float, float] | None = None
    probe_pos: tuple[float, float] | None = None
    probe_vel: tuple[float, float] | None = None
    length_residuals: tuple[float, float] | None = None
    velocity_residuals: tuple[float, float] | None = None
    detail: str = ""


def _cross(ax: float, ay: float, bx: float, by: float) -> float:
    return ax * by - ay * bx


def solve_position(link: Linkage, theta: float, branch: int) -> ConfigResult:
    """Position at input angle theta; branch is +1 or -1, kept for the run."""
    if branch not in (1, -1):
        raise ValueError("branch must be +1 or -1")
    ax = link.crank * math.cos(theta)
    ay = link.crank * math.sin(theta)
    dx = link.ground - ax
    dy = -ay
    d = math.hypot(dx, dy)
    r2, r3 = link.coupler, link.rocker

    res = ConfigResult(theta=theta, omega_in=0.0, status=OK, branch=branch)
    res.a = (ax, ay)

    if d == 0.0 and r2 == r3:
        res.status = AMBIGUOUS
        res.detail = "coincident circles: configuration not unique"
        return res
    if d > r2 + r3 or d < abs(r2 - r3) or d == 0.0:
        res.status = UNREACHABLE
        res.detail = "circles separate or nested: no intersection"
        return res

    ex, ey = dx / d, dy / d
    a_proj = (r2 * r2 - r3 * r3 + d * d) / (2.0 * d)
    h_sq = r2 * r2 - a_proj * a_proj
    tangent = h_sq <= (SINGULAR_TOL * r2) ** 2
    h = math.sqrt(h_sq) if h_sq > 0.0 else 0.0

    px, py = ax + a_proj * ex, ay + a_proj * ey
    c1 = (px - h * ey, py + h * ex)
    c2 = (px + h * ey, py - h * ex)
    s1 = _cross(dx, dy, c1[0] - ax, c1[1] - ay)
    if tangent:
        res.b = c1
        res.status = SINGULAR
        res.detail = "tangent circles: unique pose, velocity undetermined"
        return res
    res.b = c1 if (s1 > 0.0) == (branch > 0) else c2
    return res


def solve_velocity(link: Linkage, cfg: ConfigResult) -> None:
    """Velocity for non-singular configs; singular/failed keep None velocities."""
    if cfg.status != OK or cfg.a is None or cfg.b is None:
        return
    ax, ay = cfg.a
    bx, by = cfg.b
    w = cfg.omega_in
    avx = -w * link.crank * math.sin(cfg.theta)
    avy = w * link.crank * math.cos(cfg.theta)
    # Constraint derivatives: (B-A).vB = (B-A).vA ; (B-O2).vB = 0
    ux, uy = bx - ax, by - ay
    vx, vy = bx - link.ground, by
    det = _cross(ux, uy, vx, vy)
    scale = math.hypot(ux, uy) * math.hypot(vx, vy)
    if abs(det) <= SINGULAR_TOL * scale:
        cfg.status = SINGULAR
        cfg.detail = "singular jacobian: velocity undetermined"
        return
    rhs1 = ux * avx + uy * avy
    vbx = (rhs1 * vy) / det
    vby = (-rhs1 * vx) / det
    cfg.b_vel = (vbx, vby)
    cfg.omega_coupler = _cross(ux, uy, vbx - avx, vby - avy) / (ux * ux + uy * uy)
    cfg.omega_rocker = _cross(vx, vy, vbx, vby) / (vx * vx + vy * vy)
    cfg.length_residuals = (
        math.hypot(ux, uy) - link.coupler,
        math.hypot(vx, vy) - link.rocker,
    )
    cfg.velocity_residuals = (
        ux * (vbx - avx) + uy * (vby - avy),
        vx * vbx + vy * vby,
    )


def solve_probe(link: Linkage, cfg: ConfigResult, u: float, v: float) -> None:
    """Probe point in coupler-local frame: origin A, x along coupler, y left 90 deg."""
    if cfg.a is None or cfg.b is None:
        return
    ax, ay = cfg.a
    bx, by = cfg.b
    ux, uy = bx - ax, by - ay
    L = math.hypot(ux, uy)
    e1x, e1y = ux / L, uy / L
    e2x, e2y = -e1y, e1x
    cfg.probe_pos = (ax + u * e1x + v * e2x, ay + u * e1y + v * e2y)
    if cfg.status != OK or cfg.omega_coupler is None:
        cfg.probe_vel = None
        return
    w = cfg.omega_in
    avx = -w * link.crank * math.sin(cfg.theta)
    avy = w * link.crank * math.cos(cfg.theta)
    wc = cfg.omega_coupler
    cfg.probe_vel = (
        avx + wc * (u * e2x - v * e1x),
        avy + wc * (u * e2y - v * e1y),
    )


def analyze(link: Linkage, theta: float, omega_in: float, branch: int,
            probe: tuple[float, float] | None = None) -> ConfigResult:
    cfg = solve_position(link, theta, branch)
    cfg.omega_in = omega_in
    solve_velocity(link, cfg)
    if probe is not None:
        solve_probe(link, cfg, probe[0], probe[1])
    return cfg
