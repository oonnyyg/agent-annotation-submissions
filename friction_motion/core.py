"""Event-driven simulation of a horizontal slider with dry (Coulomb) friction.

Model: pure horizontal translation. Normal force N = m * g. Static friction
coefficient mu_s >= mu_k >= 0. External force is piecewise constant over
contiguous half-open intervals [t_start, t_end) covering [0, t_end].
"""

from __future__ import annotations

from dataclasses import dataclass, field


class ValidationError(ValueError):
    """Raised when a scenario definition is invalid."""


@dataclass(frozen=True)
class ForceSegment:
    t_start: float
    t_end: float
    value: float  # external force, Newtons


@dataclass(frozen=True)
class Scenario:
    name: str
    mass: float          # kg
    gravity: float       # m/s^2
    mu_static: float
    mu_kinetic: float
    x0: float            # m
    v0: float            # m/s
    t_end: float         # s
    dt: float            # sampling interval, s
    forces: tuple[ForceSegment, ...]


def _require_number(value, key):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValidationError(f"'{key}' must be a number, got {value!r}")
    return float(value)


def scenario_from_dict(data: dict) -> Scenario:
    if not isinstance(data, dict):
        raise ValidationError("scenario must be a JSON object")
    required = ["mass", "gravity", "mu_static", "mu_kinetic",
                "x0", "v0", "t_end", "dt", "forces"]
    for key in required:
        if key not in data:
            raise ValidationError(f"missing required key '{key}'")

    mass = _require_number(data["mass"], "mass")
    gravity = _require_number(data["gravity"], "gravity")
    mu_s = _require_number(data["mu_static"], "mu_static")
    mu_k = _require_number(data["mu_kinetic"], "mu_kinetic")
    x0 = _require_number(data["x0"], "x0")
    v0 = _require_number(data["v0"], "v0")
    t_end = _require_number(data["t_end"], "t_end")
    dt = _require_number(data["dt"], "dt")

    if mass <= 0:
        raise ValidationError("mass must be positive")
    if gravity <= 0:
        raise ValidationError("gravity must be positive")
    if mu_k < 0 or mu_s < 0:
        raise ValidationError("friction coefficients must be non-negative")
    if mu_s < mu_k:
        raise ValidationError("mu_static must be >= mu_kinetic")
    if t_end <= 0:
        raise ValidationError("t_end must be positive")
    if dt <= 0:
        raise ValidationError("dt must be positive")

    raw_forces = data["forces"]
    if not isinstance(raw_forces, list) or not raw_forces:
        raise ValidationError("'forces' must be a non-empty list")
    forces = []
    for i, item in enumerate(raw_forces):
        if not isinstance(item, dict):
            raise ValidationError(f"forces[{i}] must be an object")
        for key in ("t_start", "t_end", "value"):
            if key not in item:
                raise ValidationError(f"forces[{i}] missing '{key}'")
        seg = ForceSegment(
            t_start=_require_number(item["t_start"], f"forces[{i}].t_start"),
            t_end=_require_number(item["t_end"], f"forces[{i}].t_end"),
            value=_require_number(item["value"], f"forces[{i}].value"),
        )
        if seg.t_end <= seg.t_start:
            raise ValidationError(f"forces[{i}] has non-positive duration")
        forces.append(seg)

    eps = 1e-12 * max(1.0, t_end)
    if abs(forces[0].t_start) > eps:
        raise ValidationError("force segments must start at t=0")
    for prev, nxt in zip(forces, forces[1:]):
        if abs(prev.t_end - nxt.t_start) > eps:
            raise ValidationError(
                f"force segments must be contiguous (gap/overlap at t={prev.t_end})")
    if abs(forces[-1].t_end - t_end) > eps:
        raise ValidationError("force segments must end exactly at t_end")

    return Scenario(
        name=str(data.get("name", "scenario")),
        mass=mass, gravity=gravity, mu_static=mu_s, mu_kinetic=mu_k,
        x0=x0, v0=v0, t_end=t_end, dt=dt, forces=tuple(forces),
    )


@dataclass
class _Piece:
    """Analytic trajectory piece: x(t) = x0 + v0*(t-t0) + 0.5*a*(t-t0)^2."""
    t0: float
    t1: float
    x0: float
    v0: float
    a: float  # 0.0 for rest pieces

    def state_at(self, t: float) -> tuple[float, float]:
        tau = t - self.t0
        return (self.x0 + self.v0 * tau + 0.5 * self.a * tau * tau,
                self.v0 + self.a * tau)


def _force_at(forces: tuple[ForceSegment, ...], t: float) -> float:
    for seg in forces:
        if seg.t_start <= t < seg.t_end:
            return seg.value
    return forces[-1].value


def _next_boundary(forces: tuple[ForceSegment, ...], t: float) -> float:
    for seg in forces:
        if seg.t_start <= t < seg.t_end:
            return seg.t_end
    return forces[-1].t_end


def simulate(sc: Scenario) -> dict:
    """Run the event-driven simulation. Returns trajectory pieces, events,
    and cumulative energy/distance totals (independent of sampling dt)."""
    normal = sc.mass * sc.gravity
    f_s_max = sc.mu_static * normal
    f_k = sc.mu_kinetic * normal
    eps = 1e-12 * max(1.0, sc.t_end)

    pieces: list[_Piece] = []
    events: list[dict] = []
    t, x, v = 0.0, sc.x0, sc.v0
    moving = v != 0.0
    distance = 0.0
    work_force = 0.0
    dissipation = 0.0

    if not moving and abs(_force_at(sc.forces, 0.0)) <= f_s_max + eps:
        events.append({"t": 0.0, "type": "hold",
                       "force": _force_at(sc.forces, 0.0)})
    elif moving:
        events.append({"t": 0.0, "type": "start", "direction": 1 if v > 0 else -1,
                       "force": _force_at(sc.forces, 0.0), "initial": True})

    while t < sc.t_end - eps:
        force = _force_at(sc.forces, t)
        boundary = min(_next_boundary(sc.forces, t), sc.t_end)

        if not moving:
            if abs(force) <= f_s_max + eps:
                # Remain at rest until the force changes.
                pieces.append(_Piece(t, boundary, x, 0.0, 0.0))
                t = boundary
                continue
            direction = 1 if force > 0 else -1
            events.append({"t": t, "type": "start", "direction": direction,
                           "force": force})
            moving = True

        if v > 0:
            direction = 1
        elif v < 0:
            direction = -1
        else:
            direction = 1 if force > 0 else -1
        accel = (force - direction * f_k) / sc.mass
        # Time for velocity to reach zero under this acceleration.
        t_stop = None
        if accel * direction < 0:
            t_stop = t + abs(v) / abs(accel) if v != 0 else t
        t1 = boundary if t_stop is None else min(boundary, t_stop)

        dt_seg = t1 - t
        dx = v * dt_seg + 0.5 * accel * dt_seg * dt_seg
        pieces.append(_Piece(t, t1, x, v, accel))
        x += dx
        v += accel * dt_seg
        distance += abs(dx)
        work_force += force * dx
        dissipation += f_k * abs(dx)
        t = t1

        if t_stop is not None and t_stop <= boundary + eps:
            v = 0.0
            moving = False
            events.append({"t": t, "type": "stop"})
            # Re-evaluate at the same instant with the force in force_at(t):
            # if it exceeds static friction the slider restarts immediately,
            # possibly in the opposite direction, with no spurious motion.

    ke0 = 0.5 * sc.mass * sc.v0 * sc.v0
    ke1 = 0.5 * sc.mass * v * v
    residual = work_force - dissipation - (ke1 - ke0)
    return {
        "pieces": pieces,
        "events": events,
        "final": {"t": sc.t_end, "x": x, "v": v},
        "energy": {
            "work_force": work_force,
            "friction_dissipation": dissipation,
            "kinetic_initial": ke0,
            "kinetic_final": ke1,
            "kinetic_change": ke1 - ke0,
            "residual": residual,
        },
        "distance": distance,
        "net_displacement": x - sc.x0,
    }
