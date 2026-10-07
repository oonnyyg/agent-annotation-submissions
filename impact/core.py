"""Event-driven simulation of spinning rigid disks in a rectangle.

No fixed timestep: the next disk-disk or disk-wall impact time is solved
analytically, impulses are applied instantaneously, and free flight is
linear. Walls are static (infinite mass).
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from . import vec

# Tolerances (see README for the full discussion).
TIME_TOL = 1e-9          # events within this window are simultaneous
GAP_TOL = 1e-9           # allowed penetration / separation slack
APPROACH_TOL = 1e-12     # relative normal speed below this is "not approaching"


class SimError(Exception):
    """Raised for invalid input or unsupported simultaneous contacts."""


@dataclass
class Disk:
    x: float
    y: float
    vx: float
    vy: float
    omega: float
    mass: float
    radius: float

    @property
    def pos(self) -> vec.Vec:
        return (self.x, self.y)

    @property
    def vel(self) -> vec.Vec:
        return (self.vx, self.vy)

    @property
    def inertia(self) -> float:
        return 0.5 * self.mass * self.radius ** 2

    def advance(self, dt: float) -> None:
        self.x += self.vx * dt
        self.y += self.vy * dt

    def kinetic_energy(self) -> float:
        return 0.5 * self.mass * (self.vx ** 2 + self.vy ** 2) \
            + 0.5 * self.inertia * self.omega ** 2


@dataclass
class SimConfig:
    width: float
    height: float
    restitution: float
    friction: float
    t_end: float
    dt_sample: float
    max_events: int = 10000


@dataclass
class Contact:
    """A predicted impact between disk i and disk j (j == -1 means a wall)."""
    time: float
    i: int
    j: int
    wall: str | None = None  # 'left' | 'right' | 'bottom' | 'top'

    def key(self) -> tuple:
        return (self.i, self.j, self.wall)


def validate(disks: list[Disk], cfg: SimConfig) -> None:
    if not (0.0 <= cfg.restitution <= 1.0):
        raise SimError("restitution must be in [0, 1]")
    if cfg.friction < 0.0:
        raise SimError("friction coefficient must be non-negative")
    if cfg.width <= 0 or cfg.height <= 0:
        raise SimError("domain width/height must be positive")
    for k, d in enumerate(disks):
        if d.mass <= 0 or d.radius <= 0:
            raise SimError(f"disk {k}: mass and radius must be positive")
        if not (d.radius - GAP_TOL <= d.x <= cfg.width - d.radius + GAP_TOL
                and d.radius - GAP_TOL <= d.y <= cfg.height - d.radius + GAP_TOL):
            raise SimError(f"disk {k}: outside or crossing the boundary")
    for a in range(len(disks)):
        for b in range(a + 1, len(disks)):
            dist = vec.norm(vec.sub(disks[a].pos, disks[b].pos))
            rsum = disks[a].radius + disks[b].radius
            if dist < rsum - GAP_TOL:
                raise SimError(f"disks {a} and {b} overlap initially")


def _disk_disk_time(a: Disk, b: Disk) -> float | None:
    """Earliest t > 0 with |pa - pb| = ra + rb, or None."""
    dp = vec.sub(a.pos, b.pos)
    dv = vec.sub(a.vel, b.vel)
    rsum = a.radius + b.radius
    aa = vec.dot(dv, dv)
    if aa < APPROACH_TOL ** 2:
        return None  # parallel or stationary relative motion
    bb = 2.0 * vec.dot(dp, dv)
    cc = vec.dot(dp, dp) - rsum ** 2
    if bb >= 0.0 and cc > 0.0:
        return None  # separating and not touching
    disc = bb * bb - 4.0 * aa * cc
    if disc < 0.0:
        return None
    sq = math.sqrt(disc)
    for t in ((-bb - sq) / (2.0 * aa), (-bb + sq) / (2.0 * aa)):
        if t > TIME_TOL:
            return t
    return None


def _disk_wall_times(d: Disk, cfg: SimConfig) -> list[tuple[float, str]]:
    out: list[tuple[float, str]] = []
    r = d.radius
    if d.vx < -APPROACH_TOL:
        out.append(((r - d.x) / d.vx, "left"))
    elif d.vx > APPROACH_TOL:
        out.append(((cfg.width - r - d.x) / d.vx, "right"))
    if d.vy < -APPROACH_TOL:
        out.append(((r - d.y) / d.vy, "bottom"))
    elif d.vy > APPROACH_TOL:
        out.append(((cfg.height - r - d.y) / d.vy, "top"))
    return [(t, w) for t, w in out if t > TIME_TOL]


def next_contacts(disks: list[Disk], cfg: SimConfig,
                  horizon: float) -> list[Contact]:
    """All contacts at the earliest impact time (within TIME_TOL)."""
    best: list[Contact] = []
    best_t = math.inf
    for i, d in enumerate(disks):
        for t, wall in _disk_wall_times(d, cfg):
            if t < best_t - TIME_TOL:
                best, best_t = [Contact(t, i, -1, wall)], t
            elif t < best_t + TIME_TOL:
                best.append(Contact(t, i, -1, wall))
    for i in range(len(disks)):
        for j in range(i + 1, len(disks)):
            t = _disk_disk_time(disks[i], disks[j])
            if t is None:
                continue
            if t < best_t - TIME_TOL:
                best, best_t = [Contact(t, i, j)], t
            elif t < best_t + TIME_TOL:
                best.append(Contact(t, i, j))
    return [c for c in best if c.time <= horizon + TIME_TOL]


def _contact_velocity(d: Disk, r: vec.Vec) -> vec.Vec:
    """Velocity of the material point at offset r from the center."""
    return vec.add(d.vel, vec.scale(vec.perp(r), d.omega))


def _apply_impulse(d: Disk, r: vec.Vec, j: vec.Vec, sign: float) -> None:
    d.vx += sign * j[0] / d.mass
    d.vy += sign * j[1] / d.mass
    d.omega += sign * vec.cross(r, j) / d.inertia


def resolve_contact(disks: list[Disk], cfg: SimConfig,
                    c: Contact) -> dict:
    """Apply normal restitution + clamped Coulomb friction impulse."""
    a = disks[c.i]
    if c.j >= 0:
        b = disks[c.j]
        n = vec.sub(a.pos, b.pos)
        dist = vec.norm(n)
        n = (n[0] / dist, n[1] / dist)  # from j toward i
        r_a = vec.scale(n, -a.radius)
        r_b = vec.scale(n, b.radius)
        inv_ma, inv_mb = 1.0 / a.mass, 1.0 / b.mass
        inv_ia, inv_ib = 1.0 / a.inertia, 1.0 / b.inertia
        v_rel = vec.sub(_contact_velocity(a, r_a), _contact_velocity(b, r_b))
    else:
        b = None
        n = {"left": (1.0, 0.0), "right": (-1.0, 0.0),
             "bottom": (0.0, 1.0), "top": (0.0, -1.0)}[c.wall]
        r_a = vec.scale(n, -a.radius)
        r_b = (0.0, 0.0)
        inv_ma, inv_mb = 1.0 / a.mass, 0.0
        inv_ia, inv_ib = 1.0 / a.inertia, 0.0
        v_rel = _contact_velocity(a, r_a)

    t = vec.perp(n)
    ke_before = sum(d.kinetic_energy() for d in disks)

    kn = (inv_ma + inv_mb
          + vec.cross(r_a, n) ** 2 * inv_ia
          + vec.cross(r_b, n) ** 2 * inv_ib)
    vn = vec.dot(v_rel, n)
    jn = -(1.0 + cfg.restitution) * vn / kn
    jn = max(jn, 0.0)

    kt = (inv_ma + inv_mb
          + vec.cross(r_a, t) ** 2 * inv_ia
          + vec.cross(r_b, t) ** 2 * inv_ib)
    vt = vec.dot(v_rel, t)
    jt = -vt / kt  # impulse that would eliminate sliding
    limit = cfg.friction * jn
    jt = max(-limit, min(limit, jt))

    j = vec.add(vec.scale(n, jn), vec.scale(t, jt))
    _apply_impulse(a, r_a, j, +1.0)
    if b is not None:
        _apply_impulse(b, r_b, j, -1.0)

    ke_after = sum(d.kinetic_energy() for d in disks)
    return {
        "bodies": [c.i] if c.j < 0 else [c.i, c.j],
        "wall": c.wall,
        "normal": list(n),
        "impulse_normal": jn,
        "impulse_tangent": jt,
        "impulse": [j[0], j[1]],
        "ke_before": ke_before,
        "ke_after": ke_after,
        "ke_delta": ke_after - ke_before,
    }


@dataclass
class Simulation:
    disks: list[Disk]
    cfg: SimConfig
    events: list[dict] = field(default_factory=list)
    samples: list[dict] = field(default_factory=list)
    completed: bool = False
    stopped_reason: str | None = None
    initial_momentum: list[float] = field(default_factory=list)
    initial_ke: float = 0.0

    def _snapshot(self, t: float) -> dict:
        return {
            "t": t,
            "disks": [
                {"x": d.x, "y": d.y, "vx": d.vx, "vy": d.vy,
                 "omega": d.omega}
                for d in self.disks
            ],
        }

    def _check_simultaneous(self, contacts: list[Contact]) -> None:
        seen: dict[int, Contact] = {}
        for c in contacts:
            for idx in ((c.i,) if c.j < 0 else (c.i, c.j)):
                if idx in seen:
                    rel = [
                        {"disk": k, "contacts": [
                            ({"wall": cc.wall} if cc.j < 0
                             else {"disks": [cc.i, cc.j]})
                            for cc in contacts if cc.i == k or cc.j == k]}
                        for k in {x.i for x in contacts}
                        | {x.j for x in contacts if x.j >= 0}
                        if sum(1 for cc in contacts
                               if cc.i == k or cc.j == k) > 1
                    ]
                    raise SimError(
                        "simultaneous contacts share disk(s); ambiguous "
                        f"multi-point impact at t={contacts[0].time:.12g}: "
                        f"{rel}")
                seen[idx] = c

    def run(self) -> "Simulation":
        cfg = self.cfg
        t = 0.0
        self.initial_momentum = [
            sum(d.mass * d.vx for d in self.disks),
            sum(d.mass * d.vy for d in self.disks)]
        self.initial_ke = sum(d.kinetic_energy() for d in self.disks)
        n_samples = int(math.floor(cfg.t_end / cfg.dt_sample + TIME_TOL))
        sample_times = [k * cfg.dt_sample for k in range(n_samples + 1)]
        if sample_times[-1] < cfg.t_end - TIME_TOL:
            sample_times.append(cfg.t_end)
        self.samples.append(self._snapshot(0.0))

        for ts in sample_times[1:]:
            while True:
                contacts = next_contacts(self.disks, cfg, ts - t)
                if not contacts:
                    break
                te = contacts[0].time
                for d in self.disks:
                    d.advance(te)
                t += te
                self._check_simultaneous(contacts)
                record = {"t": t, "contacts": []}
                for c in contacts:
                    record["contacts"].append(
                        resolve_contact(self.disks, cfg, c))
                self.events.append(record)
                if len(self.events) >= cfg.max_events:
                    self.stopped_reason = (
                        f"event limit {cfg.max_events} reached at "
                        f"t={t:.12g}; simulation NOT complete")
                    return self
            for d in self.disks:
                d.advance(ts - t)
            t = ts
            self.samples.append(self._snapshot(t))

        self.completed = True
        return self

    def diagnostics(self) -> dict:
        p = [sum(d.mass * d.vx for d in self.disks),
             sum(d.mass * d.vy for d in self.disks)]
        wall_impulse = [0.0, 0.0]
        for ev in self.events:
            for c in ev["contacts"]:
                if c["wall"] is not None:
                    wall_impulse[0] += c["impulse"][0]
                    wall_impulse[1] += c["impulse"][1]
        ke_deltas = [c["ke_delta"] for ev in self.events
                     for c in ev["contacts"]]
        return {
            "completed": self.completed,
            "stopped_reason": self.stopped_reason,
            "event_count": len(self.events),
            "total_linear_momentum": p,
            "initial_linear_momentum": self.initial_momentum,
            "wall_impulse_total": wall_impulse,
            # With no wall contacts, momentum must equal the initial
            # momentum; wall_impulse_total reconstructs it otherwise.
            "kinetic_energy_initial": self.initial_ke,
            "kinetic_energy_final": sum(
                d.kinetic_energy() for d in self.disks),
            "kinetic_energy_change_from_events": sum(ke_deltas),
            "max_ke_gain_in_event": max(ke_deltas, default=0.0),
        }


def run(disks: list[Disk], cfg: SimConfig) -> Simulation:
    validate(disks, cfg)
    return Simulation(disks, cfg).run()
