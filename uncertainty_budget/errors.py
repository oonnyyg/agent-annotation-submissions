class BudgetError(Exception):
    """Base class for user-facing errors (bad input, bad expression, math)."""


class InputError(BudgetError):
    """Invalid input quantities or correlation matrix."""


class ExpressionError(BudgetError):
    """Invalid result expression (syntax, names, domain, differentiability)."""

    def __init__(self, message, result=None, position=None):
        self.result = result
        self.position = position
        loc = ""
        if result is not None:
            loc += f"result '{result}'"
        if position is not None:
            loc += f" at column {position}"
        if loc:
            message = f"{message} ({loc})"
        super().__init__(message)
