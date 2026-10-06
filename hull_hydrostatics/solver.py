"""Equilibrium solver: vertical translation balancing buoyancy and weight."""

from __future__ import annotations

import math

from . import geometry

BISECTION_ITERATIONS = 200


def submerged_area(vertices, translation):
    """Area of the polygon (shifted by `translation` in y) below y = 0."""
    shifted = geometry.translate_polygon(vertices, translation)
    parts = geometry.clip_below_waterline(shifted)
    return sum(geometry.polygon_area(p) for p in parts)


def solve_translation(vertices, target_area):
    """Find vertical translation t so submerged area equals target_area.

    Submerged area is continuous and monotonically non-increasing in t,
    from full area (t = -max(y)) down to 0 (t = -min(y)), so bisection
    brackets the unique solution.
    """
    ys = [v[1] for v in vertices]
    lo = -max(ys)  # fully submerged
    hi = -min(ys)  # fully emerged
    total = geometry.polygon_area(vertices)
    if target_area <= 0.0 or target_area >= total:
        raise ValueError("target submerged area outside (0, total area)")
    for _ in range(BISECTION_ITERATIONS):
        mid = 0.5 * (lo + hi)
        if submerged_area(vertices, mid) > target_area:
            lo = mid  # too much submerged: raise the body
        else:
            hi = mid
    t = 0.5 * (lo + hi)
    residual = submerged_area(vertices, t) - target_area
    return t, residual


def analyze_angle(vertices, cg, angle_deg, mass, rho, g, moment_tol):
    """Solve equilibrium at one heel angle. Returns a result dict."""
    theta = math.radians(angle_deg)
    rot = geometry.rotate_polygon(vertices, theta)
    cg_rot = geometry.rotate_point(cg, theta)
    target_area = mass / rho  # buoyancy rho*g*A*1 == mass*g
    t, area_residual = solve_translation(rot, target_area)
    shifted = geometry.translate_polygon(rot, t)
    parts = geometry.clip_below_waterline(shifted)
    sub_area = sum(geometry.polygon_area(p) for p in parts)
    # Buoyancy centre from the actual submerged polygon(s), not the
    # full-section centroid.
    num_x = 0.0
    num_y = 0.0
    for p in parts:
        a = geometry.polygon_area(p)
        c = geometry.polygon_centroid(p)
        num_x += a * c[0]
        num_y += a * c[1]
    cb = (num_x / sub_area, num_y / sub_area)
    cg_world = (cg_rot[0], cg_rot[1] + t)
    displaced_mass = rho * sub_area
    buoyancy = displaced_mass * g
    # Moment about the centre of gravity, per unit length, CCW positive.
    moment = buoyancy * (cb[0] - cg_world[0])
    residual = buoyancy - mass * g
    if abs(moment) <= moment_tol:
        behaviour = "neutral"
    elif angle_deg != 0.0 and (moment > 0.0) != (angle_deg > 0.0):
        behaviour = "righting"
    else:
        behaviour = "aggravating"
    return {
        "angle_deg": angle_deg,
        "status": "ok",
        "translation": t,
        "buoyancy_center": [cb[0], cb[1]],
        "gravity_center": [cg_world[0], cg_world[1]],
        "displaced_mass": displaced_mass,
        "submerged_area": sub_area,
        "buoyancy": buoyancy,
        "moment_about_cg": moment,
        "equilibrium_residual": residual,
        "area_residual": area_residual,
        "behaviour": behaviour,
    }
