"""Exact genotype inference on a small pedigree at one biallelic locus.

Teaching tool only: no disease or breeding-value inference.
"""

from __future__ import annotations

import itertools
from dataclasses import dataclass, field

GENOTYPES = ("AA", "AB", "BB")
_A_COUNT = {"AA": 2, "AB": 1, "BB": 0}

MAX_INDIVIDUALS = 10


class PedigreeError(ValueError):
    """Raised for invalid pedigree structure or parameters."""


@dataclass
class Individual:
    name: str
    parents: tuple[str, str] | None = None
    observation: str | None = None


@dataclass
class Pedigree:
    allele_freq: float
    error_rate: float
    members: list[Individual] = field(default_factory=list)

    def __post_init__(self) -> None:
        if not 0.0 < self.allele_freq < 1.0:
            raise PedigreeError("allele_freq must be strictly between 0 and 1")
        if not 0.0 <= self.error_rate < 1.0:
            raise PedigreeError("error_rate must be in [0, 1)")
        if not self.members:
            raise PedigreeError("pedigree must contain at least one individual")
        if len(self.members) > MAX_INDIVIDUALS:
            raise PedigreeError(
                f"pedigree supports at most {MAX_INDIVIDUALS} individuals"
            )
        self._validate_structure()

    def _validate_structure(self) -> None:
        names = [m.name for m in self.members]
        if any(not n for n in names):
            raise PedigreeError("individual names must be non-empty")
        if len(set(names)) != len(names):
            raise PedigreeError("duplicate individual id detected")
        known = set(names)
        for ind in self.members:
            if ind.parents is not None:
                father, mother = ind.parents
                if father == mother:
                    raise PedigreeError(
                        f"individual {ind.name!r} has two identical parents"
                    )
                for parent in ind.parents:
                    if parent == ind.name:
                        raise PedigreeError(
                            f"individual {ind.name!r} cannot be its own parent"
                        )
                    if parent not in known:
                        raise PedigreeError(
                            f"parent {parent!r} of {ind.name!r} is not registered"
                        )
            if ind.observation is not None and ind.observation not in GENOTYPES:
                raise PedigreeError(
                    f"invalid observation {ind.observation!r} for {ind.name!r}"
                )
        self._check_acyclic()

    def _check_acyclic(self) -> None:
        by_name = {m.name: m for m in self.members}
        WHITE, GRAY, BLACK = 0, 1, 2
        color = {name: WHITE for name in by_name}

        def visit(name: str, stack: list[str]) -> None:
            color[name] = GRAY
            stack.append(name)
            for parent in by_name[name].parents or ():
                if color[parent] == GRAY:
                    cycle = " -> ".join(stack + [parent])
                    raise PedigreeError(f"kinship cycle detected: {cycle}")
                if color[parent] == WHITE:
                    visit(parent, stack)
            stack.pop()
            color[name] = BLACK

        for name in by_name:
            if color[name] == WHITE:
                visit(name, [])

    def index(self) -> dict[str, int]:
        return {m.name: i for i, m in enumerate(self.members)}


def _gamete_alleles(genotype: str) -> tuple[str, ...]:
    if genotype == "AA":
        return ("A", "A")
    if genotype == "BB":
        return ("B", "B")
    return ("A", "B")


def transmission_table() -> dict[tuple[str, str], dict[str, float]]:
    """P(child | father, mother) under Mendelian segregation."""
    table: dict[tuple[str, str], dict[str, float]] = {}
    for father, mother in itertools.product(GENOTYPES, repeat=2):
        probs = dict.fromkeys(GENOTYPES, 0.0)
        for allele_f in _gamete_alleles(father):
            for allele_m in _gamete_alleles(mother):
                child = "".join(sorted((allele_f, allele_m)))
                probs[child] += 0.25
        table[(father, mother)] = probs
    return table


_TRANSMISSION = transmission_table()


def founder_prior(allele_freq: float) -> dict[str, float]:
    """Hardy-Weinberg prior for an independent founder."""
    p = allele_freq
    q = 1.0 - p
    return {"AA": p * p, "AB": 2.0 * p * q, "BB": q * q}


def observation_likelihoods(error_rate: float) -> dict[str, dict[str, float]]:
    """P(observed | true) for each true genotype."""
    e = error_rate
    return {
        true: {
            obs: (1.0 - e) if obs == true else e / 2.0 for obs in GENOTYPES
        }
        for true in GENOTYPES
    }


@dataclass
class InferenceResult:
    posterior: dict[str, dict[str, float]]
    evidence: float
    joint: dict[tuple[str, ...], float] = field(repr=False)
    order: list[str] = field(repr=False)

    @property
    def contradictory(self) -> bool:
        return self.evidence == 0.0


def infer(pedigree: Pedigree) -> InferenceResult:
    """Exact enumeration of the joint state space with normalization."""
    n = len(pedigree.members)
    idx = pedigree.index()
    prior = founder_prior(pedigree.allele_freq)
    obs_like = observation_likelihoods(pedigree.error_rate)

    joint: dict[tuple[str, ...], float] = {}
    for state in itertools.product(GENOTYPES, repeat=n):
        prob = 1.0
        for i, member in enumerate(pedigree.members):
            genotype = state[i]
            if member.parents is None:
                prob *= prior[genotype]
            else:
                f, m = member.parents
                prob *= _TRANSMISSION[(state[idx[f]], state[idx[m]])][genotype]
            if prob == 0.0:
                break
            if member.observation is not None:
                prob *= obs_like[genotype][member.observation]
            if prob == 0.0:
                break
        if prob > 0.0:
            joint[state] = prob

    evidence = sum(joint.values())
    order = [m.name for m in pedigree.members]
    if evidence == 0.0:
        # Contradictory observations: report honestly, never fake a posterior.
        return InferenceResult(
            posterior={name: dict.fromkeys(GENOTYPES, 0.0) for name in order},
            evidence=0.0,
            joint={},
            order=order,
        )

    posterior: dict[str, dict[str, float]] = {}
    for i, member in enumerate(pedigree.members):
        dist = dict.fromkeys(GENOTYPES, 0.0)
        for state, prob in joint.items():
            dist[state[i]] += prob
        posterior[member.name] = {
            g: dist[g] / evidence for g in GENOTYPES
        }
    return InferenceResult(
        posterior=posterior, evidence=evidence, joint=joint, order=order
    )


def predict_offspring(
    pedigree: Pedigree, result: InferenceResult, father: str, mother: str
) -> dict[str, float]:
    """Predict a hypothetical child's genotype from the parents' JOINT posterior.

    The parents' marginal posteriors must not simply be multiplied: related
    parents have correlated genotypes. The original pedigree is untouched and
    the child is never conditioned on as an observation.
    """
    if father == mother:
        raise PedigreeError("offspring prediction requires two distinct parents")
    idx = pedigree.index()
    for parent in (father, mother):
        if parent not in idx:
            raise PedigreeError(f"unknown parent for prediction: {parent!r}")
    if result.evidence == 0.0:
        raise PedigreeError(
            "cannot predict offspring: observations are contradictory "
            "(evidence probability is zero)"
        )
    fi, mi = idx[father], idx[mother]
    prediction = dict.fromkeys(GENOTYPES, 0.0)
    for state, prob in result.joint.items():
        cond = _TRANSMISSION[(state[fi], state[mi])]
        for genotype in GENOTYPES:
            prediction[genotype] += prob * cond[genotype]
    total = sum(prediction.values())
    return {g: prediction[g] / total for g in GENOTYPES}


def load_pedigree(data: dict) -> Pedigree:
    """Build a Pedigree from a JSON-decoded mapping."""
    if not isinstance(data, dict):
        raise PedigreeError("pedigree JSON must be an object")
    try:
        allele_freq = float(data["allele_freq"])
    except (KeyError, TypeError, ValueError):
        raise PedigreeError("missing or invalid 'allele_freq'") from None
    error_rate = float(data.get("error_rate", 0.0))
    raw_members = data.get("individuals")
    if not isinstance(raw_members, list):
        raise PedigreeError("'individuals' must be a list")
    members: list[Individual] = []
    for raw in raw_members:
        if not isinstance(raw, dict) or "id" not in raw:
            raise PedigreeError("each individual needs an 'id'")
        parents = raw.get("parents")
        if parents is not None:
            if (
                not isinstance(parents, list)
                or len(parents) != 2
                or not all(isinstance(p, str) for p in parents)
            ):
                raise PedigreeError(
                    f"'parents' of {raw['id']!r} must be a list of two ids"
                )
            parents = (parents[0], parents[1])
        members.append(
            Individual(
                name=str(raw["id"]),
                parents=parents,
                observation=raw.get("observation"),
            )
        )
    return Pedigree(
        allele_freq=allele_freq, error_rate=error_rate, members=members
    )
