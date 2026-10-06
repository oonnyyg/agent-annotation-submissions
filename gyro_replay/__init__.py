"""Gyroscope record replay to 3D attitude.

Quaternions are Hamilton, scalar-first (w, x, y, z), and represent the
active rotation from the body frame to the world frame.  All frames are
right-handed.  A constant body-frame angular velocity over one segment
produces an exact rotation increment that is right-multiplied onto the
current attitude: q_new = q * dq.
"""

from .core import (
    Quaternion,
    AttitudeReplay,
    load_gyro_csv,
    load_bias_json,
    load_reference_csv,
)

__all__ = [
    "Quaternion",
    "AttitudeReplay",
    "load_gyro_csv",
    "load_bias_json",
    "load_reference_csv",
]
