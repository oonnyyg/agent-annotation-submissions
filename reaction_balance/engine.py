"""Execute validated operations against the inventory, step by step."""

from __future__ import annotations

from fractions import Fraction

from .formula import parse_formula
from .model import Operation, Reaction, StepRecord


class ExecutionError(RuntimeError):
    """Raised when an operation cannot be executed."""


def run_operations(
    reactions: list[Reaction], operations: list[Operation]
) -> list[StepRecord]:
    """Run all operations; every step is checked for element conservation."""
    by_name = {reaction.name: reaction for reaction in reactions}
    inventory: dict[str, Fraction] = {}
    fed_elements: dict[str, Fraction] = {}
    steps: list[StepRecord] = []

    for index, op in enumerate(operations):
        if op.kind == "add":
            assert op.species is not None and op.amount is not None
            inventory[op.species] = inventory.get(op.species, Fraction(0)) + op.amount
            for elem, count in parse_formula(op.species).items():
                fed_elements[elem] = fed_elements.get(elem, Fraction(0)) + count * op.amount
            record = StepRecord(
                index=index,
                kind="add",
                description=f"add {op.amount} mol {op.species}",
                produced={op.species: op.amount},
                inventory=dict(inventory),
            )
        else:
            assert op.reaction is not None and op.fraction is not None
            reaction = by_name[op.reaction]
            record = _run_reaction(index, reaction, op.fraction, inventory)
        _check_conservation(inventory, fed_elements, index)
        steps.append(record)
    return steps


def _run_reaction(
    index: int,
    reaction: Reaction,
    fraction: Fraction,
    inventory: dict[str, Fraction],
) -> StepRecord:
    coeffs = reaction.coefficients
    # Maximum extent is set by the limiting reactant(s).
    max_extent: Fraction | None = None
    for species in reaction.reactants:
        available = inventory.get(species, Fraction(0))
        extent = available / coeffs[species]
        if max_extent is None or extent < max_extent:
            max_extent = extent
    assert max_extent is not None
    extent = max_extent * fraction

    limiting = [
        species
        for species in reaction.reactants
        if inventory.get(species, Fraction(0)) == coeffs[species] * max_extent
    ]
    consumed: dict[str, Fraction] = {}
    produced: dict[str, Fraction] = {}
    for species in reaction.reactants:
        amount = coeffs[species] * extent
        consumed[species] = amount
        remaining = inventory.get(species, Fraction(0)) - amount
        if remaining < 0:
            raise ExecutionError(f"inventory of {species} would become negative")
        inventory[species] = remaining
    for species in reaction.products:
        amount = coeffs[species] * extent
        produced[species] = amount
        inventory[species] = inventory.get(species, Fraction(0)) + amount
    return StepRecord(
        index=index,
        kind="react",
        description=(
            f"react {reaction.name} at fraction {fraction} "
            f"(extent {extent} mol)"
        ),
        consumed=consumed,
        produced=produced,
        limiting_reactants=limiting,
        inventory=dict(inventory),
    )


def _check_conservation(
    inventory: dict[str, Fraction],
    fed_elements: dict[str, Fraction],
    index: int,
) -> None:
    totals: dict[str, Fraction] = {}
    for species, amount in inventory.items():
        if amount == 0:
            continue
        for elem, count in parse_formula(species).items():
            totals[elem] = totals.get(elem, Fraction(0)) + count * amount
    if totals != {k: v for k, v in fed_elements.items() if v != 0}:
        raise ExecutionError(
            f"element conservation check failed at step {index}"
        )