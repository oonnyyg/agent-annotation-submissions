"""Hydrostatic-equilibrium storage model for layered van Genuchten soils.

All lengths are in centimetres. Depth z is measured downward from the
surface (z = 0). For a water table at depth ``d`` the suction head at
depth ``z`` is ``h = max(d - z, 0)``; at and below the water table the
soil is saturated. No infiltration dynamics are simulated.
"""

from __future__ import annotations

import math
from dataclasses import dataclass

_MAX_RECURSION = 40


class IntegrationError(RuntimeError):
    """Raised when adaptive quadrature fails to meet the tolerance."""


@dataclass(frozen=True)
class Layer:
    """One soil layer with van Genuchten parameters.

    thickness: layer thickness in cm (> 0)
    theta_r:   residual water content, 0 <= theta_r < theta_s <= 1
    theta_s:   saturated water content
    alpha:     scale parameter in 1/cm (> 0)
    n:         shape parameter (> 1); m = 1 - 1/n
    """

    thickness: float
    theta_r: float
    theta_s: float
    alpha: float
    n: float

    def __post_init__(self) -> None:
        vals = (self.thickness, self.theta_r, self.theta_s, self.alpha, self.n)
        if not all(math.isfinite(v) for v in vals):
            raise ValueError("layer parameters must be finite")
        if self.thickness <= 0.0:
            raise ValueError("layer thickness must be positive")
        if not (0.0 <= self.theta_r < self.theta_s <= 1.0):
            raise ValueError("require 0 <= theta_r < theta_s <= 1")
        if self.alpha <= 0.0:
            raise ValueError("alpha must be positive (1/cm)")
        if self.n <= 1.0:
            raise ValueError("n must be greater than 1")

    @property
    def m(self) -> float:
        return 1.0 - 1.0 / self.n


def van_genuchten_theta(layer: Layer, suction_head: float) -> float:
    """Volumetric water content at suction head h (cm); saturated for h <= 0."""
    if not math.isfinite(suction_head):
        raise ValueError("suction head must be finite")
    if suction_head <= 0.0:
        return layer.theta_s
    base = 1.0 + (layer.alpha * suction_head) ** layer.n
    return layer.theta_r + (layer.theta_s - layer.theta_r) * base ** (-layer.m)


def _simpson(f, a: float, b: float):
    c = 0.5 * (a + b)
    fa, fb, fc = f(a), f(b), f(c)
    return fa, fb, fc, (b - a) * (fa + 4.0 * fc + fb) / 6.0


def _adaptive_simpson(f, a: float, b: float, tol: float) -> float:
    fa, fb, fc, whole = _simpson(f, a, b)

    def recurse(a, b, fa, fb, fc, whole, tol, depth):
        c = 0.5 * (a + b)
        d = 0.5 * (a + c)
        e = 0.5 * (c + b)
        fd, fe = f(d), f(e)
        left = (c - a) * (fa + 4.0 * fd + fc) / 6.0
        right = (b - c) * (fc + 4.0 * fe + fb) / 6.0
        delta = left + right - whole
        if depth <= 0:
            raise IntegrationError(
                "adaptive integration did not converge within "
                f"{_MAX_RECURSION} subdivision levels on [{a}, {b}]"
            )
        if abs(delta) <= 15.0 * tol:
            return left + right + delta / 15.0
        return recurse(a, c, fa, fc, fd, left, 0.5 * tol, depth - 1) + recurse(
            c, b, fc, fb, fe, right, 0.5 * tol, depth - 1
        )

    return recurse(a, b, fa, fb, fc, whole, tol, _MAX_RECURSION)


def _total_depth(layers) -> float:
    return sum(layer.thickness for layer in layers)


def layer_index_at(layers, depth: float) -> int:
    """Layer owning a sampling point.

    A point exactly on an internal boundary belongs to the lower layer;
    the bottom point of the profile belongs to the last layer.
    """
    total = _total_depth(layers)
    if not (0.0 <= depth <= total):
        raise ValueError(f"sample depth {depth} outside profile [0, {total}]")
    z = 0.0
    for i, layer in enumerate(layers):
        z += layer.thickness
        if depth < z or i == len(layers) - 1:
            return i
    return len(layers) - 1  # unreachable


def theta_at_depth(layers, water_table: float, depth: float) -> float:
    """Water content at a point for a given water table depth (cm)."""
    _check_water_table(water_table)
    layer = layers[layer_index_at(layers, depth)]
    return van_genuchten_theta(layer, water_table - depth)


def _check_water_table(water_table: float) -> None:
    if not math.isfinite(water_table) or water_table < 0.0:
        raise ValueError("water table depth must be finite and non-negative")


@dataclass(frozen=True)
class LayerStorage:
    layer_index: int
    top: float
    bottom: float
    storage: float  # equivalent water thickness in cm


@dataclass(frozen=True)
class ProfileStorage:
    water_table: float
    layers: tuple
    total: float
    tolerance: float
    converged: bool


def _layer_storage(layer: Layer, z_top: float, z_bot: float,
                   water_table: float, tol: float) -> float:
    """Integrate theta over [z_top, z_bot], splitting at the water table."""
    storage = 0.0
    # Unsaturated part above the water table: integrate over suction head.
    wet_top = max(z_top, 0.0)
    wet_bot = min(z_bot, water_table)
    if wet_bot > wet_top:
        h_lo = water_table - wet_bot   # suction at deeper end (>= 0)
        h_hi = water_table - wet_top   # suction at shallower end

        def theta_of_h(h):
            return van_genuchten_theta(layer, h)

        if h_lo == 0.0:
            # Split at the water table boundary: saturated sliver is exact.
            storage += _adaptive_simpson(theta_of_h, 0.0, h_hi, tol)
        else:
            storage += _adaptive_simpson(theta_of_h, h_lo, h_hi, tol)
    # Saturated part at/below the water table: theta = theta_s exactly.
    sat_bot = z_bot
    sat_top = max(z_top, water_table)
    if sat_bot > sat_top:
        storage += layer.theta_s * (sat_bot - sat_top)
    return storage


def profile_storage(layers, water_table: float,
                    tolerance: float = 1e-9) -> ProfileStorage:
    """Per-layer and total storage (cm of water) for a water table depth."""
    _check_water_table(water_table)
    if not (math.isfinite(tolerance) and tolerance > 0.0):
        raise ValueError("tolerance must be positive and finite")
    parts = []
    z = 0.0
    converged = True
    for i, layer in enumerate(layers):
        z_next = z + layer.thickness
        try:
            s = _layer_storage(layer, z, z_next, water_table, tolerance)
        except IntegrationError:
            converged = False
            raise
        parts.append(LayerStorage(i, z, z_next, s))
        z = z_next
    total = sum(p.storage for p in parts)
    return ProfileStorage(water_table, tuple(parts), total, tolerance, converged)


@dataclass(frozen=True)
class InversionResult:
    solved: bool
    water_table: float | None
    target: float
    achieved: float | None
    residual: float | None
    min_storage: float  # storage with water table at max_depth
    max_storage: float  # storage with water table at 0 (fully saturated)
    message: str


def invert_water_table(layers, target: float, max_depth: float,
                       tolerance: float = 1e-9,
                       bracket_tol: float = 1e-10) -> InversionResult:
    """Shallowest water table in [0, max_depth] matching target storage.

    Total storage decreases monotonically as the water table deepens, so
    bisection finds the shallowest match. If the target lies outside the
    achievable range, no boundary value is returned as a fake result.
    """
    if not (math.isfinite(target) and target >= 0.0):
        raise ValueError("target storage must be finite and non-negative")
    if not (math.isfinite(max_depth) and max_depth > 0.0):
        raise ValueError("max_depth must be positive and finite")
    s_shallow = profile_storage(layers, 0.0, tolerance).total
    s_deep = profile_storage(layers, max_depth, tolerance).total
    if target > s_shallow or target < s_deep:
        return InversionResult(
            False, None, target, None, None, s_deep, s_shallow,
            "target outside achievable storage range "
            f"[{s_deep:.6g}, {s_shallow:.6g}] cm",
        )
    lo, hi = 0.0, max_depth  # storage(lo) >= target >= storage(hi)
    for _ in range(200):
        mid = 0.5 * (lo + hi)
        s_mid = profile_storage(layers, mid, tolerance).total
        if s_mid > target:
            lo = mid
        else:
            hi = mid
        if hi - lo <= bracket_tol * max(1.0, max_depth):
            break
    depth = lo  # shallowest depth whose storage still meets the target
    achieved = profile_storage(layers, depth, tolerance).total
    return InversionResult(
        True, depth, target, achieved, achieved - target, s_deep, s_shallow,
        "ok",
    )
