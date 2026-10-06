class UncertaintyBudgetError(Exception):
    """Base error for user-facing failures."""


class InputSpecError(UncertaintyBudgetError):
    """Invalid JSON specification (inputs, correlation, results)."""


class ExpressionError(UncertaintyBudgetError):
    """Invalid expression syntax or semantics."""

    def __init__(self, message, position=None):
        self.position = position
        if position is not None:
            message = f"{message} (at column {position + 1})"
        super().__init__(message)


class EvaluationError(UncertaintyBudgetError):
    """Domain error or non-differentiable point at the nominal values."""
