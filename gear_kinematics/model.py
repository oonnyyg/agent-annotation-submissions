"""Parse the gear-train JSON document and build linear speed constraints.

All constraints are linear equations in the per-axis signed speeds (rpm),
with a common positive direction for every axis.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from .errors import InputError


@dataclass
class Constraint:
    source: str                 # connection or case input identifier
    kind: str                   # rigid / external_mesh / internal_mesh / planetary / input
    coeffs: dict                # axis name -> coefficient
    rhs: float
    description: str


@dataclass
class Case:
    name: str
    inputs: list                # list[Constraint]
    torques: dict               # axis -> N*m


@dataclass
class GearTrain:
    axes: list                  # ordered axis names
    constraints: list           # structural constraints from connections
    cases: list                 # list[Case]


def _require(cond, msg):
    if not cond:
        raise InputError(msg)


def _teeth(value, where):
    _require(isinstance(value, int) and not isinstance(value, bool) and value > 0,
             f"{where}: teeth must be a positive integer, got {value!r}")
    return value


def _gear_ref(node, where):
    _require(isinstance(node, dict), f"{where}: expected object with axis/teeth")
    axis = node.get("axis")
    _require(isinstance(axis, str), f"{where}: missing axis name")
    return axis, _teeth(node.get("teeth"), where)


def _build_connection(conn, index, axes_set):
    _require(isinstance(conn, dict), f"connections[{index}]: expected object")
    ctype = conn.get("type")
    cid = conn.get("id", f"conn[{index}]")
    _require(isinstance(cid, str), f"connections[{index}]: id must be a string")

    def check_axis(axis):
        _require(axis in axes_set, f"{cid}: unknown axis {axis!r}")

    if ctype == "rigid":
        pair = conn.get("axes")
        _require(isinstance(pair, list) and len(pair) == 2,
                 f"{cid}: rigid connection needs exactly two axes")
        a, b = pair
        check_axis(a); check_axis(b)
        _require(a != b, f"{cid}: rigid connection needs two distinct axes")
        return [Constraint(cid, ctype, {a: 1.0, b: -1.0}, 0.0,
                           f"rigid: {a} == {b}")]

    if ctype in ("external_mesh", "internal_mesh"):
        ga = conn.get("gear_a")
        gb = conn.get("gear_b")
        if ctype == "internal_mesh" and "ring" in conn:
            ga = conn.get("ring")
            gb = conn.get("pinion")
        aa, za = _gear_ref(ga, f"{cid}.gear_a")
        ab, zb = _gear_ref(gb, f"{cid}.gear_b")
        check_axis(aa); check_axis(ab)
        _require(aa != ab, f"{cid}: meshing gears must be on distinct axes")
        if ctype == "internal_mesh":
            _require(za > zb, f"{cid}: internal mesh requires ring teeth > pinion teeth")
            sign = -1.0   # same direction of rotation
        else:
            sign = 1.0    # opposite directions
        return [Constraint(cid, ctype, {aa: float(za), ab: sign * float(zb)}, 0.0,
                           f"{ctype}: {aa}(z={za}) vs {ab}(z={zb})")]

    if ctype == "planetary":
        sa, zs = _gear_ref(conn.get("sun"), f"{cid}.sun")
        ra, zr = _gear_ref(conn.get("ring"), f"{cid}.ring")
        carrier = conn.get("carrier")
        _require(isinstance(carrier, str), f"{cid}: missing carrier axis")
        for axis in (sa, ra, carrier):
            check_axis(axis)
        _require(len({sa, ra, carrier}) == 3,
                 f"{cid}: sun, ring and carrier must be distinct axes")
        zplanet = _teeth(conn.get("planet_teeth"), f"{cid}.planet_teeth")
        _require(zr == zs + 2 * zplanet,
                 f"{cid}: geometry impossible, need ring teeth == sun + 2*planet "
                 f"({zs} + 2*{zplanet} != {zr})")
        # Willis equation: (ws - wc)*zs + (wr - wc)*zr = 0
        coeffs = {sa: float(zs), ra: float(zr), carrier: float(-(zs + zr))}
        return [Constraint(cid, ctype, coeffs, 0.0,
                           f"planetary: sun z={zs}, ring z={zr}, planet z={zplanet}")]

    raise InputError(f"{cid}: unknown connection type {ctype!r}")


def _build_case(node, index, axes_set):
    _require(isinstance(node, dict), f"cases[{index}]: expected object")
    name = node.get("name", f"case[{index}]")
    _require(isinstance(name, str), f"cases[{index}]: name must be a string")
    inputs = []
    seen = set()
    for j, spec in enumerate(node.get("inputs", [])):
        _require(isinstance(spec, dict), f"{name}: inputs[{j}] expected object")
        axis = spec.get("axis")
        _require(isinstance(axis, str) and axis in axes_set,
                 f"{name}: inputs[{j}] references unknown axis {axis!r}")
        _require(axis not in seen, f"{name}: duplicate input for axis {axis!r}")
        seen.add(axis)
        if spec.get("fixed") is True:
            rpm = 0.0
        else:
            rpm = spec.get("rpm")
            _require(isinstance(rpm, (int, float)) and not isinstance(rpm, bool),
                     f"{name}: inputs[{j}] needs numeric rpm or fixed=true")
            rpm = float(rpm)
        src = f"{name}/input[{axis}]"
        inputs.append(Constraint(src, "input", {axis: 1.0}, rpm,
                                 f"input: {axis} = {rpm} rpm"))
    torques = {}
    for axis, tq in (node.get("torques") or {}).items():
        _require(axis in axes_set, f"{name}: torque on unknown axis {axis!r}")
        _require(isinstance(tq, (int, float)) and not isinstance(tq, bool),
                 f"{name}: torque for {axis} must be numeric (N*m)")
        torques[axis] = float(tq)
    return Case(name, inputs, torques)


def load_document(doc):
    _require(isinstance(doc, dict), "top-level JSON must be an object")
    axes = doc.get("axes")
    _require(isinstance(axes, list) and axes and all(isinstance(a, str) for a in axes),
             "axes must be a non-empty list of names")
    _require(len(set(axes)) == len(axes), "axis names must be unique")
    axes_set = set(axes)

    constraints = []
    for i, conn in enumerate(doc.get("connections", [])):
        constraints.extend(_build_connection(conn, i, axes_set))

    cases = [_build_case(c, i, axes_set) for i, c in enumerate(doc.get("cases", []))]
    _require(cases, "at least one case is required")
    names = [c.name for c in cases]
    _require(len(set(names)) == len(names), "case names must be unique")
    return GearTrain(axes, constraints, cases)
