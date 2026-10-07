"""Gated 2D particle tracker.

Links per-frame position observations (with possible missed detections
and crossing targets) into interpretable trajectories using a constant
velocity Kalman filter, Mahalanobis gating and global one-to-one
assignment.
"""

from .config import Config
from .tracker import Tracker, run_tracking

__all__ = ["Config", "Tracker", "run_tracking"]
