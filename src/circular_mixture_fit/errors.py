"""Error types for circular_mixture_fit."""


class InputError(Exception):
    """Raised when CSV data or JSON configuration is invalid."""


class FitError(Exception):
    """Raised when the EM fit fails numerically."""
