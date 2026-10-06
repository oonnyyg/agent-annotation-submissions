"""Validation of definitions/operations and step-by-step inventory simulation."""

from __future__ import annotations

from decimal import Decimal, InvalidOperation
from fractions import Fraction

from .balance import balance_reaction
from .formula import FormulaError, parse_formula


class DefinitionError(ValueError):
    """Raised for invalid reaction definitions or operations."""


def _parse_amount(value, what):
    if isinstance(value, bool):
        raise DefinitionError(f"{what} must be a non-negative decimal, got {value!r}")
    if isinstance(value, (int, float, str)):
        try:
            amount = Fraction(Decimal(str(value)))
        except InvalidOperation:
            raise DefinitionError(f"{what} is not a valid decimal: {value!r}") from None
    else:
        raise DefinitionError(f"{what} must be a non-negative decimal, got {value!r}")
    if amount < 0:
        raise DefinitionError(f"{what} must be non-negative, got {value!r}")
    return amount


def load_definition(data):
    """Validate the full input document; return (reactions, operations)."""
    if not isinstance(data, dict):
        raise DefinitionError("top-level JSON must be an object")
    raw_reactions = data.get("reactions")
    raw_operations = data.get("operations")
    if not isinstance(raw_reactions, list) or not raw_reactions:
        raise DefinitionError("'reactions' must be a non-empty list")
    if not isinstance(raw_operations, list):
        raise DefinitionError("'operations' must be a list")

    reactions = []
    names = set()
    for i, raw in enumerate(raw_reactions):
        if not isinstance(raw, dict):
            raise DefinitionError(f"reaction #{i} must be an object")
        name = raw.get("name", f"reaction_{i}")
        if not isinstance(name, str) or not name:
            raise DefinitionError(f"reaction #{i} has an invalid name")
        if name in names:
            raise DefinitionError(f"duplicate reaction name {name!r}")
        names.add(name)
        reactants = raw.get("reactants")
        products = raw.get("products")
        for key, value in (("reactants", reactants), ("products", products)):
            if not isinstance(value, list) or not all(isinstance(s, str) for s in value):
                raise DefinitionError(f"reaction {name!r}: {key} must be a list of formulas")
        coefficients = balance_reaction(reactants, products)
        reactions.append(
            {
                "name": name,
                "reactants": list(reactants),
                "products": list(products),
                "coefficients": coefficients,
            }
        )

    operations = []
    for i, raw in enumerate(raw_operations):
        if not isinstance(raw, dict):
            raise DefinitionError(f"operation #{i} must be an object")
        kind = raw.get("type")
        if kind == "add":
            species = raw.get("species")
            if not isinstance(species, str):
                raise DefinitionError(f"operation #{i}: 'species' must be a formula string")
            try:
                parse_formula(species)
            except FormulaError as exc:
                raise DefinitionError(f"operation #{i}: {exc}") from None
            amount = _parse_amount(raw.get("moles"), f"operation #{i} moles")
            operations.append({"type": "add", "species": species, "moles": amount})
        elif kind == "react":
            target = raw.get("reaction")
            if target not in names:
                raise DefinitionError(f"operation #{i}: unknown reaction {target!r}")
            fraction = _parse_amount(raw.get("fraction"), f"operation #{i} fraction")
            if fraction > 1:
                raise DefinitionError(f"operation #{i}: fraction must be between 0 and 1")
            operations.append({"type": "react", "reaction": target, "fraction": fraction})
        else:
            raise DefinitionError(f"operation #{i}: unknown type {kind!r}")
    return reactions, operations


def _element_totals(stock):
    totals = {}
    for species, amount in stock.items():
        if amount == 0:
            continue
        for el, count in parse_formula(species).items():
            totals[el] = totals.get(el, Fraction(0)) + count * amount
    return totals


def simulate(reactions, operations):
    """Run operations; return list of step records. Stock never goes negative."""
    by_name = {r["name"]: r for r in reactions}
    stock: dict[str, Fraction] = {}
    fed: dict[str, Fraction] = {}
    steps = []

    def snapshot():
        return {s: amt for s, amt in sorted(stock.items()) if amt != 0}

    steps.append(
        {"step": 0, "action": "initial", "detail": "", "changes": {},
         "limiting_reactants": [], "stock": snapshot()}
    )

    for index, op in enumerate(operations, start=1):
        changes: dict[str, Fraction] = {}
        limiting: list[str] = []
        if op["type"] == "add":
            species = op["species"]
            amount = op["moles"]
            stock[species] = stock.get(species, Fraction(0)) + amount
            changes[species] = amount
            for el, count in parse_formula(species).items():
                fed[el] = fed.get(el, Fraction(0)) + count * amount
            detail = f"add {amount} mol {species}"
        else:
            reaction = by_name[op["reaction"]]
            coeffs = reaction["coefficients"]
            ratios = []
            for reactant in reaction["reactants"]:
                available = stock.get(reactant, Fraction(0))
                ratios.append((available / coeffs[reactant], reactant))
            extent_max = min(r for r, _ in ratios)
            best = min(ratios)[0]
            limiting = sorted(name for r, name in ratios if r == best)
            extent = extent_max * op["fraction"]
            for reactant in reaction["reactants"]:
                delta = -extent * coeffs[reactant]
                stock[reactant] = stock.get(reactant, Fraction(0)) + delta
                changes[reactant] = changes.get(reactant, Fraction(0)) + delta
            for product in reaction["products"]:
                delta = extent * coeffs[product]
                stock[product] = stock.get(product, Fraction(0)) + delta
                changes[product] = changes.get(product, Fraction(0)) + delta
            for species, amount in stock.items():
                if amount < 0:
                    raise AssertionError(f"negative stock for {species}")
            detail = (
                f"react {op['reaction']} fraction={op['fraction']} "
                f"extent={extent} mol (max {extent_max} mol)"
            )
        steps.append(
            {
                "step": index,
                "action": op["type"],
                "detail": detail,
                "changes": dict(sorted(changes.items())),
                "limiting_reactants": limiting,
                "stock": snapshot(),
            }
        )
        totals = _element_totals(stock)
        if totals != {el: v for el, v in fed.items() if v != 0}:
            raise AssertionError(f"element conservation violated at step {index}")
    return steps
