"""Cell-centred finite-volume solver for 1-D retarded solute transport.

Governing equation (unit cross-section, porosity folded into parameters):

    R * dC/dt = -v * dC/dx + D * d2C/dx2 - R * lambda * C

where R >= 1 is the retardation factor and lambda >= 0 the first-order
decay rate acting on the *total* (dissolved + sorbed) mass.

Discretisation: explicit Euler in time, upwind advective fluxes,
central diffusive fluxes.  Left boundary total flux v * C_in(t);
right boundary zero diffusive flux, advective outflow v * C_last.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field


def _check_finite(name: str, value: float) -> float:
    value = float(value)
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite, got {value!r}")
    return value


@dataclass
class TransportConfig:
    """Validated simulation parameters (SI units, mg/m^3 concentrations)."""

    length: float            # domain length [m]
    cells: int               # number of uniform cells (>= 2)
    velocity: float          # pore-water velocity [m/s], >= 0
    dispersion: float        # dispersion coefficient [m^2/s], >= 0
    retardation: float       # retardation factor [-], >= 1
    decay: float             # first-order decay rate [1/s], >= 0
    inlet: list              # [(t, c), ...] piecewise-constant inlet schedule
    output_times: list       # non-decreasing output times [s]
    initial: list | None = None  # per-cell initial concentration, default 0

    @classmethod
    def from_dict(cls, data: dict) -> "TransportConfig":
        try:
            length = _check_finite("length", data["length"])
            cells = int(data["cells"])
            velocity = _check_finite("velocity", data["velocity"])
            dispersion = _check_finite("dispersion", data["dispersion"])
            retardation = _check_finite("retardation", data["retardation"])
            decay = _check_finite("decay", data["decay"])
            inlet = [( _check_finite("inlet time", t),
                       _check_finite("inlet concentration", c))
                     for t, c in data["inlet"]]
            output_times = [_check_finite("output time", t)
                            for t in data["output_times"]]
        except (KeyError, TypeError) as exc:
            raise ValueError(f"invalid configuration: {exc}") from exc

        if length <= 0.0:
            raise ValueError("length must be positive")
        if cells < 2:
            raise ValueError("cells must be at least 2")
        if velocity < 0.0:
            raise ValueError("velocity must be non-negative")
        if dispersion < 0.0:
            raise ValueError("dispersion must be non-negative")
        if retardation < 1.0:
            raise ValueError("retardation must be >= 1")
        if decay < 0.0:
            raise ValueError("decay must be non-negative")
        if not inlet:
            raise ValueError("inlet schedule must not be empty")
        if inlet[0][0] != 0.0:
            raise ValueError("inlet schedule must start at t = 0")
        for (t0, c0), (t1, c1) in zip(inlet, inlet[1:]):
            if t1 <= t0:
                raise ValueError("inlet times must be strictly increasing")
            if c0 < 0.0:
                raise ValueError("inlet concentrations must be non-negative")
        if inlet[-1][1] < 0.0:
            raise ValueError("inlet concentrations must be non-negative")
        for t0, t1 in zip(output_times, output_times[1:]):
            if t1 < t0:
                raise ValueError("output times must be non-decreasing")
        if output_times and output_times[0] < 0.0:
            raise ValueError("output times must be non-negative")

        initial = data.get("initial")
        if initial is not None:
            initial = [_check_finite("initial concentration", c) for c in initial]
            if len(initial) != cells:
                raise ValueError("initial must have one value per cell")
            if any(c < 0.0 for c in initial):
                raise ValueError("initial concentrations must be non-negative")

        return cls(length, cells, velocity, dispersion, retardation, decay,
                   inlet, output_times, initial)


def _inlet_value(schedule: list, t: float) -> float:
    """Piecewise-constant inlet concentration; last value persists."""
    value = schedule[0][1]
    for t_break, conc in schedule:
        if t_break > t:
            break
        value = conc
    return value


def _stable_dt(cfg: TransportConfig, dx: float) -> float:
    """Explicit-Euler stability limit (advection, diffusion, decay)."""
    limits = []
    # Combined explicit limit: v*dt/(R*dx) + 2*D*dt/(R*dx^2) <= 1.
    rate = 0.0
    if cfg.velocity > 0.0:
        rate += cfg.velocity / (cfg.retardation * dx)
    if cfg.dispersion > 0.0:
        rate += 2.0 * cfg.dispersion / (cfg.retardation * dx * dx)
    if rate > 0.0:
        limits.append(1.0 / rate)
    if cfg.decay > 0.0:
        limits.append(0.02 / cfg.decay)  # accuracy, not just stability
    if not limits:
        return math.inf
    return min(limits)


def run_simulation(cfg: TransportConfig) -> dict:
    """Run the simulation and return profiles, outlet curve and mass budget."""
    n = cfg.cells
    dx = cfg.length / n
    r = cfg.retardation
    lam = cfg.decay
    v = cfg.velocity
    diff = cfg.dispersion

    conc = list(cfg.initial) if cfg.initial is not None else [0.0] * n

    def stored_mass() -> float:
        return r * dx * sum(conc)

    mass_initial = stored_mass()
    mass_in = 0.0
    mass_out = 0.0
    mass_decayed = 0.0

    t_end = cfg.output_times[-1] if cfg.output_times else 0.0
    events = sorted({t for t, _ in cfg.inlet if 0.0 < t <= t_end}
                    | set(cfg.output_times))

    dt_max = _stable_dt(cfg, dx)
    profiles = []
    outlet = []
    residuals = []

    def record(t: float) -> None:
        profiles.append({"time": t, "concentration": list(conc)})
        outlet.append({"time": t, "concentration": conc[-1]})
        residual = (mass_initial + mass_in - mass_out
                    - mass_decayed - stored_mass())
        residuals.append({"time": t, "residual": residual})

    t = 0.0
    if cfg.output_times and cfg.output_times[0] == 0.0:
        record(0.0)

    for t_event in events:
        if t_event <= t:
            continue
        span = t_event - t
        steps = 1 if math.isinf(dt_max) else max(1, math.ceil(span / dt_max))
        dt = span / steps
        for _ in range(steps):
            c_in = _inlet_value(cfg.inlet, t)
            # Face fluxes F[0..n]; F[0] is the total prescribed inflow.
            flux = [v * c_in]
            for i in range(1, n):
                flux.append(v * conc[i - 1]
                            - diff * (conc[i] - conc[i - 1]) / dx)
            flux.append(v * conc[-1])  # zero diffusive flux at outlet
            decay_step = lam * r * dx * sum(conc)
            for i in range(n):
                conc[i] += dt * (-(flux[i + 1] - flux[i]) / (r * dx)
                                 - lam * conc[i])
            mass_in += dt * flux[0]
            mass_out += dt * flux[-1]
            mass_decayed += dt * decay_step
            t += dt
        if t_event in set(cfg.output_times):
            record(t_event)

    scale = max(mass_initial + mass_in, 1.0)
    return {
        "dx": dx,
        "cell_centers": [(i + 0.5) * dx for i in range(n)],
        "profiles": profiles,
        "outlet": outlet,
        "mass_balance": {
            "initial": mass_initial,
            "inflow": mass_in,
            "outflow": mass_out,
            "decayed": mass_decayed,
            "stored": stored_mass(),
            "residual": mass_initial + mass_in - mass_out
                        - mass_decayed - stored_mass(),
            "relative_residual": (mass_initial + mass_in - mass_out
                                  - mass_decayed - stored_mass()) / scale,
            "history": residuals,
        },
    }
