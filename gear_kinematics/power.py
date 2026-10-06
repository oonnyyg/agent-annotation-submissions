"""Signed shaft power from external torques and solved speeds.

Only computed when speeds are uniquely determined.
"""

from __future__ import annotations

import math

from .model import Case, GearTrain


def power_balance(train: GearTrain, case: Case, speeds_rpm: dict) -> dict:
    """Signed power per shaft (W) and total balance.

    P = T * omega with omega = rpm * 2*pi/60. Missing torques count as zero.
    Positive power means the external torque feeds energy into the train.
    """
    per_shaft = {}
    for shaft in train.shafts:
        torque = case.torques.get(shaft, 0.0)
        omega = speeds_rpm[shaft] * 2.0 * math.pi / 60.0
        per_shaft[shaft] = {
            "torque_Nm": torque,
            "speed_rpm": speeds_rpm[shaft],
            "power_W": torque * omega,
        }
    total = sum(p["power_W"] for p in per_shaft.values())
    return {"per_shaft": per_shaft, "total_power_W": total}
