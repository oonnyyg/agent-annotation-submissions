"""Error types raised by the solver and data loaders."""


class DataError(Exception):
    """Invalid or inconsistent input data."""


class GeometryError(Exception):
    """Degenerate geometry: design matrix is rank deficient."""


class ConvergenceError(Exception):
    """Iterative least squares failed to converge."""
