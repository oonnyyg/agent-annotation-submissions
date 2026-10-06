"""Input validation for the clearance tool. All errors raise ValueError."""

import math


def _num(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{name} must be a number, got {value!r}")
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite, got {value!r}")
    return float(value)


def validate_positive(value, name):
    v = _num(value, name)
    if v <= 0.0:
        raise ValueError(f"{name} must be positive, got {v}")
    return v


def validate_supports(spec):
    if not isinstance(spec, dict):
        raise ValueError("'supports' must be an object with left/right entries")
    try:
        left, right = spec["left"], spec["right"]
        x0 = _num(left["x"], "supports.left.x")
        y0 = _num(left["y"], "supports.left.y")
        x1 = _num(right["x"], "supports.right.x")
        y1 = _num(right["y"], "supports.right.y")
    except (KeyError, TypeError) as exc:
        raise ValueError(f"missing support field: {exc}") from exc
    if not x1 > x0:
        raise ValueError("right support x must be strictly greater than left support x")
    return (x0, y0), (x1, y1)


def validate_terrain(points, x0, x1):
    if not isinstance(points, list) or len(points) < 2:
        raise ValueError("terrain must be a list of at least two [x, y] points")
    pts = []
    for i, p in enumerate(points):
        if not isinstance(p, (list, tuple)) or len(p) != 2:
            raise ValueError(f"terrain point {i} must be [x, y]")
        pts.append((_num(p[0], f"terrain[{i}].x"), _num(p[1], f"terrain[{i}].y")))
    for i in range(1, len(pts)):
        if not pts[i][0] > pts[i - 1][0]:
            raise ValueError("terrain x coordinates must be strictly increasing")
    if pts[0][0] > x0 or pts[-1][0] < x1:
        raise ValueError("terrain polyline must cover the whole span [x0, x1]")
    return pts


def validate_case(case, index, chord):
    if not isinstance(case, dict):
        raise ValueError(f"case {index} must be an object")
    name = case.get("name", f"case_{index}")
    if not isinstance(name, str):
        raise ValueError(f"case[{index}].name must be a string")
    length = validate_positive(case.get("length"), f"case[{index}].length")
    if not length > chord:
        raise ValueError(
            f"cable length {length} must be strictly greater than the "
            f"support chord distance {chord}"
        )
    return name, length
