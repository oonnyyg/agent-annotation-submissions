class ConfigError(ValueError):
    """Raised when the JSON configuration is invalid.

    The message always includes a location such as ``spins[1].t2``.
    """
