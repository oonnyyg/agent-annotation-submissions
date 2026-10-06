"""Scenario analysis: solve each cable length independently and judge limits."""

from __future__ import annotations

from .clearance import clearance_against_terrain, validate_terrain
from .solver import CatenaryError, solve_catenary


def analyze_scenario(length, x1, y1, x2, y2, weight_per_length, terrain,
                     max_tension=None, min_clearance=None):
    """Analyze one cable length; failures never affect other scenarios."""
    result = {"length": length, "ok": False, "checks": {}}
    try:
        cat = solve_catenary(x1, y1, x2, y2, length, weight_per_length)
    except CatenaryError as exc:
        result["error"] = str(exc)
        return result
    clr = clearance_against_terrain(cat, terrain)
    t_left, t_right = cat.support_tensions
    vx, vy = cat.span_lowest_point()
    result.update(
        ok=True,
        a=cat.a,
        horizontal_tension=cat.horizontal_tension,
        tension_left=t_left,
        tension_right=t_right,
        max_tension=cat.max_tension,
        vertex={
            "x": cat.x_vertex,
            "y": cat.y_vertex,
            "inside_span": cat.x1 <= cat.x_vertex <= cat.x2,
        },
        span_lowest_point={"x": vx, "y": vy},
        clearance={
            "min": clr.min_clearance,
            "x": clr.x,
            "cable_y": clr.cable_y,
            "terrain_y": clr.terrain_y,
            "collision": clr.collision,
        },
        residuals=cat.residuals(),
    )
    checks = result["checks"]
    passed = True
    if max_tension is not None:
        ok_t = cat.max_tension <= max_tension
        checks["max_tension"] = {
            "limit": max_tension,
            "value": cat.max_tension,
            "pass": ok_t,
        }
        passed = passed and ok_t
    if min_clearance is not None:
        ok_c = clr.min_clearance >= min_clearance
        checks["min_clearance"] = {
            "limit": min_clearance,
            "value": clr.min_clearance,
            "pass": ok_c,
        }
        passed = passed and ok_c
    result["pass"] = passed
    return result


def analyze_config(config):
    """Run all scenarios from a parsed JSON config dict."""
    span = config["span"]
    x1, y1 = float(span["x1"]), float(span["y1"])
    x2, y2 = float(span["x2"]), float(span["y2"])
    density = float(config["linear_density"])
    gravity = float(config.get("gravity", 9.81))
    if density <= 0.0:
        raise CatenaryError("linear_density must be positive")
    if gravity <= 0.0:
        raise CatenaryError("gravity must be positive")
    weight = density * gravity
    terrain = [(float(p[0]), float(p[1])) for p in config["terrain"]]
    validate_terrain(terrain, x1, x2)
    limits = config.get("limits", {})
    max_tension = limits.get("max_tension")
    min_clearance = limits.get("min_clearance")
    scenarios = [
        analyze_scenario(
            float(length), x1, y1, x2, y2, weight, terrain,
            max_tension, min_clearance,
        )
        for length in config["lengths"]
    ]
    return {
        "span": {"x1": x1, "y1": y1, "x2": x2, "y2": y2},
        "linear_density": density,
        "gravity": gravity,
        "weight_per_length": weight,
        "limits": limits,
        "scenarios": scenarios,
    }
