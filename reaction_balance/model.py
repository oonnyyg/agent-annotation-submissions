"""Core model: validated reaction definitions and stepwise operations."""

from __future__ import annotations

from dataclasses import dataclass, field
from decimal import Decimal, InvalidOperation
from fractions import Fraction

from .balance import balance_equation
from .formula import parse_formula


class DefinitionError(ValueError):
    """Raised when the reaction/operation definitions are invalid."""


def parse_amount(value: object, what: str) -> Fraction:
    """Parse a decimal amount (string or JSON number) into an exact Fraction."""
    if isinstance(value, bool):
        raise DefinitionError(f"{what} must be a decimal number, got {value!r}")
    if isinstance(value, (int, float, str)):
        try:
            decimal = Decimal(str(value))
        except InvalidOperation:
            raise DefinitionError(f"{what} is not a valid decimal: {value!r}") from None
    else:
        raise DefinitionError(f"{what} must be a decimal number, got {value!r}")
    if not decimal.is_finite():
        raise DefinitionError(f"{what} must be finite, got {value!r}")
    return Fraction(decimal)


@dataclass
class Reaction:
    name: str
    reactants: list[str]
    products: list[str]
    coefficients: dict[str, int]


@dataclass
class Operation:
    kind: str  # "add" or "react"
    species: str | None = None
    amount: Fraction | None = None
    reaction: str | None = None
    fraction: Fraction | None = None


@dataclass
class StepRecord:
    index: int
    kind: str
    description: str
    consumed: dict[str, Fraction] = field(default_factory=dict)
    produced: dict[str, Fraction] = field(default_factory=dict)
    limiting_reactants: list[str] = field(default_factory=list)
    inventory: dict[str, Fraction] = field(default_factory=dict)


def load_definition(data: object) -> tuple[list[Reaction], list[Operation]]:
    """Validate the full JSON definition before anything is executed."""
    if not isinstance(data, dict):
        raise DefinitionError("top-level JSON value must be an object")
    raw_reactions = data.get("reactions")
    raw_operations = data.get("operations")
    if not isinstance(raw_reactions, list) or not raw_reactions:
        raise DefinitionError("'reactions' must be a non-empty array")
    if not isinstance(raw_operations, list):
        raise DefinitionError("'operations' must be an array")

    reactions: list[Reaction] = []
    names: set[str] = set()
    for idx, raw in enumerate(raw_reactions):
        where = f"reactions[{idx}]"
        if not isinstance(raw, dict):
            raise DefinitionError(f"{where} must be an object")
        name = raw.get("name")
        if not isinstance(name, str) or not name:
            raise DefinitionError(f"{where}.name must be a non-empty string")
        if name in names:
            raise DefinitionError(f"duplicate reaction name: {name!r}")
        names.add(name)
        reactants = raw.get("reactants")
        products = raw.get("products")
        for key, value in (("reactants", reactants), ("products", products)):
            if (
                not isinstance(value, list)
                or not value
                or not all(isinstance(s, str) for s in value)
            ):
                raise DefinitionError(
                    f"{where}.{key} must be a non-empty array of formulas"
                )
        coefficients = balance_equation(reactants, products)
        reactions.append(Reaction(name, list(reactants), list(products), coefficients))

    operations: list[Operation] = []
    for idx, raw in enumerate(raw_operations):
        where = f"operations[{idx}]"
        if not isinstance(raw, dict):
            raise DefinitionError(f"{where} must be an object")
        kind = raw.get("type")
        if kind == "add":
            species = raw.get("species")
            if not isinstance(species, str) or not species:
                raise DefinitionError(f"{where}.species must be a formula string")
            parse_formula(species)  # validate formula syntax and elements
            if "amount" not in raw:
                raise DefinitionError(f"{where}.amount is required")
            amount = parse_amount(raw["amount"], f"{where}.amount")
            if amount < 0:
                raise DefinitionError(f"{where}.amount must be non-negative")
            operations.append(Operation("add", species=species, amount=amount))
        elif kind == "react":
            reaction = raw.get("reaction")
            if not isinstance(reaction, str) or reaction not in names:
                raise DefinitionError(
                    f"{where}.reaction must name a defined reaction"
                )
            if "fraction" not in raw:
                raise DefinitionError(f"{where}.fraction is required")
            fraction = parse_amount(raw["fraction"], f"{where}.fraction")
            if not 0 <= fraction <= 1:
                raise DefinitionError(f"{where}.fraction must be between 0 and 1")
            operations.append(
                Operation("react", reaction=reaction, fraction=fraction)
            )
        else:
            raise DefinitionError(f"{where}.type must be 'add' or 'react'")
    return reactions, operations