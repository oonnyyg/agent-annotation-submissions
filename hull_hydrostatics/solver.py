"""Equilibrium solver: vertical translation and righting-moment analysis.

For each heel angle the section and center of gravity are rotated
counterclockwise, then a vertical translation is found so that the
buoyancy from the submerged area (below world y = 0) equals the weight.
"""

from __future__ import annotations

import math

from .geometry import (
    area_centroid,
    clip_below_waterline,
    rotate_points,
    translate_points,
)

_BISECTION_ITERATIONS = 200


class SolveError(RuntimeError):
    """Raised when equilibrium cannot be found for an angle."""


def _submerged_area(rotated, translation):
    shifted = translate_points(rotated, 0.0, translation)
    submerged = clip_below_waterline(shifted)
    area, _ = area_centroid(submerged)
    return area


def solve_angle(rotated_section, rotated_cg, target_area):
    """Solve the vertical translation for one rotated section.

    Returns a dict with translation, submerged area/centroid (buoyancy
    center), and the buoyancy residual against the target displacement.
    """
    ys = [p[1] for p in rotated_section]
    y_min = min(ys)
    y_max = max(ys)
    # translation t: body moves up by t. Fully submerged when y_max + t <= 0,
    # fully out of the water when y_min + t >= 0.
    t_low = -y_max  # submerged area = full section area
    t_high = -y_min  # submerged area = 0
    full_area, _ = area_centroid(rotated_section)
    if not (0.0 < target_area < full_area):
        raise SolveError("target displacement outside (0, full displacement)")

    for _ in range(_BISECTION_ITERATIONS):
        t_mid = 0.5 * (t_low + t_high)
        area = _submerged_area(rotated_section, t_mid)
        if area > target_area:
            t_low = t_mid
        else:
            t_high = t_mid
    translation = 0.5 * (t_low + t_high)

    shifted = translate_points(rotated_section, 0.0, translation)
    submerged = clip_below_waterline(shifted)
    area, centroid = area_centroid(submerged)
    if area <= 0.0:
        raise SolveError("no submerged area at equilibrium")
    cg_world = (rotated_cg[0], rotated_cg[1] + translation)
    return {
        "translation": translation,
        "submerged_area": area,
        "buoyancy_center": centroid,
        "gravity_center": cg_world,
        "residual_area": area - target_area,
    }


def analyze_angle(section_ccw, cg, angle_deg, target_area, rho, gravity,
                  moment_tolerance):
    """Full per-angle analysis. Returns a result dict (status ok/failed)."""
    angle_rad = math.radians(angle_deg)
    rotated = rotate_points(section_ccw, angle_rad)
    rotated_cg = rotate_points([cg], angle_rad)[0]
    try:
        sol = solve_angle(rotated, rotated_cg, target_area)
    except SolveError as exc:
        return {"angle_deg": angle_deg, "status": "failed", "error": str(exc)}

    buoyancy = rho * gravity * sol["submerged_area"]
    weight = rho * gravity * target_area
    bx, by = sol["buoyancy_center"]
    gx, gy = sol["gravity_center"]
    # Moment about the center of gravity, CCW positive: M = F_y * (Bx - Gx).
    moment = buoyancy * (bx - gx)

    if abs(moment) <= moment_tolerance:
        classification = "neutral"
    elif angle_deg != 0.0 and (moment > 0) != (angle_deg > 0):
        classification = "righting"
    elif angle_deg == 0.0:
        classification = "neutral" if abs(moment) <= moment_tolerance else "unclassified"
    else:
        classification = "upsetting"

    return {
        "angle_deg": angle_deg,
        "status": "ok",
        "translation": sol["translation"],
        "buoyancy_center": [bx, by],
        "gravity_center": [gx, gy],
        "displaced_mass": rho * sol["submerged_area"],
        "buoyancy": buoyancy,
        "weight": weight,
        "residual": buoyancy - weight,
        "moment_about_cg": moment,
        "classification": classification,
    }
