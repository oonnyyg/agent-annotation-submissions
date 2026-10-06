"""Two-stage flash separation material balance tool."""

from .flash import FlashResult, solve_flash
from .cascade import StageResult, TwoStageResult, run_two_stage

__all__ = [
    "FlashResult",
    "solve_flash",
    "StageResult",
    "TwoStageResult",
    "run_two_stage",
]
