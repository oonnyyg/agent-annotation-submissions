"""Annual maxima Gumbel extreme-value analysis (standard library only)."""

from .core import (
    EULER_MASCHERONI,
    AnnualSeries,
    FitResult,
    build_annual_series,
    empirical_exceedance,
    fit_gumbel,
    gumbel_quantile,
)

__all__ = [
    "EULER_MASCHERONI",
    "AnnualSeries",
    "FitResult",
    "build_annual_series",
    "empirical_exceedance",
    "fit_gumbel",
    "gumbel_quantile",
]
