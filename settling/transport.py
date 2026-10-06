"""Depth-translation transport, sampling concentrations, mass balance."""
# Depth z is measured downward from the water surface; the bed is at
# z = column.depth. Each class starts uniformly distributed over [0, H]
# with linear density mass/H (kg per metre). After time t the suspended
# slab occupies [v*t, H]; material that crossed the bed is deposited.
import math


def _check_time(time):
    if not math.isfinite(time) or time < 0.0:
        raise ValueError(f"time must be non-negative and finite, got {time!r}")


def suspended_mass(initial_mass, velocity, time, depth):
    """Remaining suspended mass (kg) of one class at the given time."""
    _check_time(time)
    settled = velocity * time
    fraction = max(0.0, (depth - settled) / depth)
    return initial_mass * fraction


def deposited_mass(initial_mass, velocity, time, depth):
    """Cumulative deposited mass (kg); suspended + deposited == initial."""
    return initial_mass - suspended_mass(initial_mass, velocity, time, depth)


def interval_concentration(initial_mass, velocity, time, column, z_top, z_bottom):
    """Average mass concentration (kg/m^3) of one class in [z_top, z_bottom]."""
    _check_time(time)
    depth = column.depth
    if not (0.0 <= z_top < z_bottom <= depth):
        raise ValueError(
            f"sampling interval [{z_top}, {z_bottom}] must lie inside the "
            f"column (0, {depth}) with positive thickness"
        )
    slab_top = min(velocity * time, depth)
    overlap = max(0.0, min(z_bottom, depth) - max(z_top, slab_top))
    mass_in_interval = initial_mass * overlap / depth
    volume = column.area * (z_bottom - z_top)
    return mass_in_interval / volume


def mass_balance(classes, velocities, time, depth):
    """Per-class and total suspended/deposited masses at the given time."""
    _check_time(time)
    rows = []
    total_initial = total_suspended = total_deposited = 0.0
    for cls, velocity in zip(classes, velocities):
        suspended = suspended_mass(cls.mass, velocity, time, depth)
        deposited = cls.mass - suspended
        rows.append(
            {
                "name": cls.name,
                "initial": cls.mass,
                "suspended": suspended,
                "deposited": deposited,
                "closure_error": cls.mass - suspended - deposited,
            }
        )
        total_initial += cls.mass
        total_suspended += suspended
        total_deposited += deposited
    totals = {
        "name": "TOTAL",
        "initial": total_initial,
        "suspended": total_suspended,
        "deposited": total_deposited,
        "closure_error": total_initial - total_suspended - total_deposited,
    }
    return rows, totals
