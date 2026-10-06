"""Run all cable-length cases against limits and build the report."""

import math

from .clearance import min_clearance, terrain_y
from .solver import CatenarySolution, SolveError
from .validate import (
    validate_case,
    validate_positive,
    validate_supports,
    validate_terrain,
)

DEFAULT_SAMPLES = 201


def load_config(cfg):
    if not isinstance(cfg, dict):
        raise ValueError("top-level JSON value must be an object")
    (x0, y0), (x1, y1) = validate_supports(cfg.get("supports", {}))
    density = validate_positive(cfg.get("linear_density"), "linear_density")
    gravity = validate_positive(cfg.get("gravity"), "gravity")
    terrain = validate_terrain(cfg.get("terrain"), x0, x1)
    cases = cfg.get("cases")
    if not isinstance(cases, list) or not cases:
        raise ValueError("'cases' must be a non-empty list")
    samples = cfg.get("samples", DEFAULT_SAMPLES)
    if isinstance(samples, bool) or not isinstance(samples, int) or samples < 2:
        raise ValueError("samples must be an integer >= 2")
    limits = {}
    for key in ("max_tension", "min_clearance"):
        v = cfg.get(key)
        if v is not None:
            limits[key] = validate_positive(v, key)
    return {
        "supports": ((x0, y0), (x1, y1)),
        "weight_per_length": density * gravity,
        "terrain": terrain,
        "cases": cases,
        "samples": samples,
        "limits": limits,
    }


def analyze_case(name, length, ctx):
    (x0, y0), (x1, y1) = ctx["supports"]
    result = {"name": name, "length": length}
    try:
        sol = CatenarySolution(x0, y0, x1, y1, length, ctx["weight_per_length"])
    except SolveError as exc:
        result.update(solved=False, error=str(exc))
        return result, None

    t_left = sol.tension(x0)
    t_right = sol.tension(x1)
    clearance = min_clearance(sol, ctx["terrain"])
    vx, vy = sol.vertex
    lx, ly = sol.lowest_point_in_span()

    checks = {}
    ok = True
    limits = ctx["limits"]
    if "max_tension" in limits:
        passed = max(t_left, t_right) <= limits["max_tension"]
        checks["max_tension"] = {"limit": limits["max_tension"], "pass": passed}
        ok &= passed
    if "min_clearance" in limits:
        passed = clearance["min_clearance"] >= limits["min_clearance"]
        checks["min_clearance"] = {"limit": limits["min_clearance"], "pass": passed}
        ok &= passed

    result.update(
        solved=True,
        parameter_a=sol.a,
        horizontal_tension=sol.horizontal_tension,
        tension_left=t_left,
        tension_right=t_right,
        max_tension=max(t_left, t_right),
        vertex={"x": vx, "y": vy, "within_span": x0 <= vx <= x1},
        lowest_point_in_span={"x": lx, "y": ly},
        clearance=clearance,
        residuals=sol.residuals(),
        checks=checks,
        passed=ok,
    )
    return result, sol


def sample_rows(sol, terrain, samples):
    x0, x1 = sol.x0, sol.x1
    step = (x1 - x0) / (samples - 1)
    rows = []
    for i in range(samples):
        x = x0 + i * step
        ty = terrain_y(terrain, x)
        cy = sol.y(x)
        rows.append((x, cy, ty, cy - ty))
    return rows


def run(config):
    ctx = load_config(config)
    (x0, y0), (x1, y1) = ctx["supports"]
    chord = math.hypot(x1 - x0, y1 - y0)
    cases_out = []
    solutions = {}
    for i, case in enumerate(ctx["cases"]):
        try:
            name, length = validate_case(case, i, chord)
        except ValueError as exc:
            bad = case.get("name", f"case_{i}") if isinstance(case, dict) else f"case_{i}"
            cases_out.append({"name": bad, "solved": False, "error": str(exc)})
            continue
        result, sol = analyze_case(name, length, ctx)
        cases_out.append(result)
        if sol is not None:
            solutions[name] = sol
    report = {
        "supports": {"left": {"x": x0, "y": y0}, "right": {"x": x1, "y": y1}},
        "span": x1 - x0,
        "chord": chord,
        "weight_per_length": ctx["weight_per_length"],
        "limits": ctx["limits"],
        "cases": cases_out,
    }
    return report, solutions, ctx
