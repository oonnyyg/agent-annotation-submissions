"""Event-driven 2D rigid-disk impact simulation (stdlib only).

Disks are uniform solid cylinders (I = 1/2 m r^2). Walls are static and
of infinite mass. Free flight is linear; collision times are solved
analytically (no fixed-step tunnelling).
"""

from __future__ import annotations

import math
from dataclasses import dataclass

# --- numerical tolerances (see README) ---
TIME_TOL = 1e-9          # events within this window are simultaneous
APPROACH_TOL = 1e-9      # |normal velocity| below this is "grazing"
GEOM_TOL = 1e-9          # overlap / boundary slack for validation


class SimError(Exception):
    """Invalid input or configuration."""


class MultiContactError(SimError):
    """Simultaneous contacts sharing a disk: ambiguous, abort."""

    def __init__(self, contacts):
        self.contacts = contacts
        msg = "simultaneous contacts share a disk: " + "; ".join(
            c["description"] for c in contacts
        )
        super().__init__(msg)


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
    def inertia(self) -> float:
        return 0.5 * self.mass * self.radius * self.radius

    def advance(self, dt: float) -> None:
        self.x += self.vx * dt
        self.y += self.vy * dt

    def kinetic_energy(self) -> float:
        return 0.5 * self.mass * (self.vx ** 2 + self.vy ** 2) + \
            0.5 * self.inertia * self.omega ** 2

    def surface_velocity(self, rx: float, ry: float):
        """Velocity of the material point at offset (rx, ry) from centre."""
        return (self.vx - self.omega * ry, self.vy + self.omega * rx)


def disk_disk_time(a: Disk, b: Disk):
    """Earliest t > TIME_TOL with |pa - pb| = ra + rb, or None.

    Grazing (non-approaching) contacts return None.
    """
    dx = b.x - a.x
    dy = b.y - a.y
    dvx = b.vx - a.vx
    dvy = b.vy - a.vy
    rs = a.radius + b.radius
    aa = dvx * dvx + dvy * dvy
    if aa < APPROACH_TOL ** 2:
        return None
    bb = 2.0 * (dx * dvx + dy * dvy)
    if bb >= -APPROACH_TOL:
        return None  # separating or grazing
    cc = dx * dx + dy * dy - rs * rs
    disc = bb * bb - 4.0 * aa * cc
    if disc <= 0.0:
        return None
    t = (-bb - math.sqrt(disc)) / (2.0 * aa)
    return t if t > TIME_TOL else None


def disk_wall_time(d: Disk, width: float, height: float):
    """Earliest (t, wall) with wall in {'left','right','bottom','top'}."""
    best = None
    candidates = (
        ("left", d.radius - d.x, d.vx),
        ("right", (width - d.radius) - d.x, d.vx),
        ("bottom", d.radius - d.y, d.vy),
        ("top", (height - d.radius) - d.y, d.vy),
    )
    for name, gap, v in candidates:
        # gap is signed distance to the wall contact plane along the axis.
        if name in ("left", "bottom"):
            if v >= -APPROACH_TOL:
                continue
            t = gap / v
        else:
            if v <= APPROACH_TOL:
                continue
            t = gap / v
        if t is not None and t > TIME_TOL and (best is None or t < best[0]):
            best = (t, name)
    return best


def _apply_impulse(d: Disk, jx: float, jy: float, rx: float, ry: float,
                   sign: float) -> None:
    """Apply impulse (sign * j) at contact offset (rx, ry) on disk d."""
    d.vx += sign * jx / d.mass
    d.vy += sign * jy / d.mass
    d.omega += sign * (rx * jy - ry * jx) / d.inertia


def resolve_disk_disk(a: Disk, b: Disk, restitution: float, friction: float):
    """Impulse exchange between disks a and b; returns event record fields."""
    dx = b.x - a.x
    dy = b.y - a.y
    dist = math.hypot(dx, dy)
    nx, ny = dx / dist, dy / dist          # normal: a -> b
    tx, ty = -ny, nx                        # tangent
    # contact-point offsets from each centre
    rax, ray = a.radius * nx, a.radius * ny
    rbx, rby = -b.radius * nx, -b.radius * ny
    ua = a.surface_velocity(rax, ray)
    ub = b.surface_velocity(rbx, rby)
    rvx, rvy = ub[0] - ua[0], ub[1] - ua[1]
    vn = rvx * nx + rvy * ny
    vt = rvx * tx + rvy * ty
    kn = 1.0 / a.mass + 1.0 / b.mass  # offsets parallel to n: no spin term
    kt = kn + a.radius ** 2 / a.inertia + b.radius ** 2 / b.inertia
    jn = -(1.0 + restitution) * vn / kn
    jt = -vt / kt
    limit = friction * jn
    jt = max(-limit, min(limit, jt))
    jx, jy = jn * nx + jt * tx, jn * ny + jt * ty
    _apply_impulse(a, jx, jy, rax, ray, -1.0)
    _apply_impulse(b, jx, jy, rbx, rby, +1.0)
    return {"jn": jn, "jt": jt, "normal": [nx, ny]}


WALL_NORMALS = {
    "left": (1.0, 0.0),
    "right": (-1.0, 0.0),
    "bottom": (0.0, 1.0),
    "top": (0.0, -1.0),
}


def resolve_disk_wall(d: Disk, wall: str, restitution: float, friction: float):
    """Impulse exchange between disk and static wall (infinite mass)."""
    nx, ny = WALL_NORMALS[wall]             # inward normal
    tx, ty = -ny, nx
    rx, ry = -d.radius * nx, -d.radius * ny
    ux, uy = d.surface_velocity(rx, ry)
    vn = ux * nx + uy * ny
    vt = ux * tx + uy * ty
    kn = 1.0 / d.mass
    kt = kn + d.radius ** 2 / d.inertia
    jn = -(1.0 + restitution) * vn / kn
    jt = -vt / kt
    limit = friction * jn
    jt = max(-limit, min(limit, jt))
    jx, jy = jn * nx + jt * tx, jn * ny + jt * ty
    _apply_impulse(d, jx, jy, rx, ry, +1.0)
    return {"jn": jn, "jt": jt, "normal": [nx, ny]}


@dataclass
class SimConfig:
    width: float
    height: float
    restitution: float
    friction: float
    t_end: float
    sample_dt: float
    max_events: int = 10000


class Simulation:
    def __init__(self, config: SimConfig, disks):
        self.config = config
        self.disks = list(disks)
        self.time = 0.0
        self.events = []
        self.samples = []
        self.completed = False
        self._validate()

    def _validate(self) -> None:
        c = self.config
        if not (0.0 <= c.restitution <= 1.0):
            raise SimError("restitution must be in [0, 1]")
        if c.friction < 0.0:
            raise SimError("friction must be non-negative")
        if c.width <= 0 or c.height <= 0:
            raise SimError("domain dimensions must be positive")
        if c.sample_dt <= 0 or c.t_end < 0:
            raise SimError("sample_dt must be positive and t_end non-negative")
        for i, d in enumerate(self.disks):
            if d.mass <= 0 or d.radius <= 0:
                raise SimError(f"disk {i}: mass and radius must be positive")
            if not (d.radius - GEOM_TOL <= d.x <= c.width - d.radius + GEOM_TOL
                    and d.radius - GEOM_TOL <= d.y
                    <= c.height - d.radius + GEOM_TOL):
                raise SimError(f"disk {i}: outside domain or crossing a wall")
        for i in range(len(self.disks)):
            for j in range(i + 1, len(self.disks)):
                a, b = self.disks[i], self.disks[j]
                dist = math.hypot(b.x - a.x, b.y - a.y)
                if dist < a.radius + b.radius - GEOM_TOL:
                    raise SimError(f"disks {i} and {j}: initial overlap")

    # -- event detection ------------------------------------------------
    def _next_events(self):
        """All contacts within TIME_TOL of the earliest collision time."""
        best = None
        for i, d in enumerate(self.disks):
            hit = disk_wall_time(d, self.config.width, self.config.height)
            if hit and (best is None or hit[0] < best):
                best = hit[0]
            for j in range(i + 1, len(self.disks)):
                t = disk_disk_time(d, self.disks[j])
                if t is not None and (best is None or t < best):
                    best = t
        if best is None:
            return None
        contacts = []
        for i, d in enumerate(self.disks):
            hit = disk_wall_time(d, self.config.width, self.config.height)
            if hit and hit[0] <= best + TIME_TOL:
                contacts.append({"kind": "wall", "disks": [i],
                                 "wall": hit[1], "time": hit[0]})
            for j in range(i + 1, len(self.disks)):
                t = disk_disk_time(d, self.disks[j])
                if t is not None and t <= best + TIME_TOL:
                    contacts.append({"kind": "disk", "disks": [i, j],
                                     "time": t})
        return best, contacts

    @staticmethod
    def _check_shared(contacts):
        seen = {}
        shared = False
        for c in contacts:
            for idx in c["disks"]:
                if idx in seen:
                    shared = True
                seen[idx] = c
        if shared:
            descs = [
                {"description": (
                    f"disk {c['disks'][0]} <-> wall {c['wall']}"
                    if c["kind"] == "wall" else
                    f"disk {c['disks'][0]} <-> disk {c['disks'][1]}"),
                 "time": c["time"], "disks": c["disks"]}
                for c in contacts
            ]
            raise MultiContactError(descs)

    # -- bookkeeping ----------------------------------------------------
    def _total_energy(self) -> float:
        return sum(d.kinetic_energy() for d in self.disks)

    def _total_momentum(self):
        return (sum(d.mass * d.vx for d in self.disks),
                sum(d.mass * d.vy for d in self.disks))

    def _snapshot(self):
        return [
            {"x": d.x, "y": d.y, "vx": d.vx, "vy": d.vy,
             "omega": d.omega}
            for d in self.disks
        ]

    def _record_sample(self, t: float) -> None:
        self.samples.append({"time": t, "disks": self._snapshot()})

    def _apply_events(self, contacts) -> None:
        ke_before = self._total_energy()
        px_before, py_before = self._total_momentum()
        applied = []
        for c in contacts:
            if c["kind"] == "wall":
                d = self.disks[c["disks"][0]]
                imp = resolve_disk_wall(
                    d, c["wall"], self.config.restitution,
                    self.config.friction)
                applied.append({"kind": "wall", "disk": c["disks"][0],
                                "wall": c["wall"], **imp})
            else:
                a = self.disks[c["disks"][0]]
                b = self.disks[c["disks"][1]]
                imp = resolve_disk_disk(
                    a, b, self.config.restitution, self.config.friction)
                applied.append({"kind": "disk",
                                "disks": list(c["disks"]), **imp})
        ke_after = self._total_energy()
        px_after, py_after = self._total_momentum()
        involves_wall = any(c["kind"] == "wall" for c in contacts)
        self.events.append({
            "time": self.time,
            "contacts": applied,
            "ke_before": ke_before,
            "ke_after": ke_after,
            "ke_delta": ke_after - ke_before,
            "momentum_before": [px_before, py_before],
            "momentum_after": [px_after, py_after],
            "momentum_delta": [px_after - px_before, py_after - py_before],
            "momentum_check": None if involves_wall else
                math.hypot(px_after - px_before, py_after - py_before),
        })

    # -- main loop --------------------------------------------------------
    def run(self):
        c = self.config
        next_sample = 0.0
        while True:
            # emit samples strictly before the next event
            t0 = self.time
            found = self._next_events()
            t_event = found[0] if found else math.inf
            while next_sample <= c.t_end + TIME_TOL and \
                    t0 + t_event > next_sample + TIME_TOL:
                for d in self.disks:
                    d.advance(next_sample - self.time)
                self.time = next_sample
                self._record_sample(self.time)
                next_sample += c.sample_dt
            if t0 + t_event > c.t_end + TIME_TOL:
                if self.time < c.t_end - TIME_TOL:
                    for d in self.disks:
                        d.advance(c.t_end - self.time)
                    self.time = c.t_end
                    self._record_sample(self.time)
                self.completed = True
                break
            if len(self.events) >= c.max_events:
                self.completed = False
                break
            _, contacts = found
            self._check_shared(contacts)
            for d in self.disks:
                d.advance(t0 + found[0] - self.time)
            self.time = t0 + found[0]
            self._apply_events(contacts)
            # sample landing exactly on the event uses post-collision state
            if abs(self.time - next_sample) <= TIME_TOL:
                self._record_sample(self.time)
                next_sample += c.sample_dt
        return self

    def diagnostics(self):
        return {
            "completed": self.completed,
            "final_time": self.time,
            "event_count": len(self.events),
            "events": self.events,
            "final_energy": self._total_energy(),
            "final_momentum": list(self._total_momentum()),
        }
