"""Pedigree genotype inference for a single biallelic locus (teaching tool).

Exact enumeration of the joint genotype state space. Genotypes are AA, AB, BB.
This tool does not infer disease risk or breeding value.
"""

from __future__ import annotations

GENOTYPES = ("AA", "AB", "BB")
# Allele counts of A per genotype index.
_A_COUNT = (2, 1, 0)


def _mendel_table():
    """Mendelian transmission: child genotype probs given parent genotypes."""
    table = {}
    for gi, a1 in enumerate(_A_COUNT):
        for gj, a2 in enumerate(_A_COUNT):
            p1 = a1 / 2.0
            p2 = a2 / 2.0
            table[(gi, gj)] = (
                p1 * p2,
                p1 * (1 - p2) + (1 - p1) * p2,
                (1 - p1) * (1 - p2),
            )
    return table


MENDEL = _mendel_table()


class PedigreeError(ValueError):
    """Raised for invalid pedigree or parameter specifications."""


def _validate(spec):
    if not isinstance(spec, dict):
        raise PedigreeError("spec must be a JSON object")
    p = spec.get("allele_frequency")
    if not isinstance(p, (int, float)) or isinstance(p, bool) or not (0.0 < p < 1.0):
        raise PedigreeError("allele_frequency must be strictly between 0 and 1")
    e = spec.get("error_rate", 0.0)
    if not isinstance(e, (int, float)) or isinstance(e, bool) or not (0.0 <= e < 1.0):
        raise PedigreeError("error_rate must be in [0, 1)")

    raw = spec.get("individuals")
    if not isinstance(raw, list) or not raw:
        raise PedigreeError("individuals must be a non-empty list")
    if len(raw) > 10:
        raise PedigreeError("at most 10 individuals are supported")

    individuals = {}
    order = []
    for entry in raw:
        if not isinstance(entry, dict) or not isinstance(entry.get("id"), str):
            raise PedigreeError("each individual needs a string id")
        iid = entry["id"]
        if iid in individuals:
            raise PedigreeError(f"duplicate individual id: {iid!r}")
        parents = entry.get("parents")
        if parents is None:
            parents = ()
        else:
            if (
                not isinstance(parents, list)
                or len(parents) != 2
                or parents[0] == parents[1]
                or not all(isinstance(x, str) for x in parents)
            ):
                raise PedigreeError(
                    f"individual {iid!r}: parents must be two distinct ids"
                )
            parents = tuple(parents)
        obs = entry.get("observation")
        if obs is not None and obs not in GENOTYPES:
            raise PedigreeError(f"individual {iid!r}: invalid observation {obs!r}")
        individuals[iid] = {"id": iid, "parents": parents, "observation": obs}
        order.append(iid)

    for iid, ind in individuals.items():
        for par in ind["parents"]:
            if par not in individuals:
                raise PedigreeError(f"individual {iid!r}: unknown parent {par!r}")

    # Cycle check via topological sort.
    indegree = {iid: 0 for iid in order}
    children = {iid: [] for iid in order}
    for iid, ind in individuals.items():
        for par in ind["parents"]:
            indegree[iid] += 1
            children[par].append(iid)
    queue = [i for i in order if indegree[i] == 0]
    topo = []
    while queue:
        node = queue.pop()
        topo.append(node)
        for ch in children[node]:
            indegree[ch] -= 1
            if indegree[ch] == 0:
                queue.append(ch)
    if len(topo) != len(order):
        raise PedigreeError("pedigree contains a relationship cycle")

    predict = spec.get("predict_offspring")
    if predict is not None:
        if (
            not isinstance(predict, list)
            or len(predict) != 2
            or predict[0] == predict[1]
            or not all(isinstance(x, str) for x in predict)
        ):
            raise PedigreeError("predict_offspring must be two distinct ids")
        for pid in predict:
            if pid not in individuals:
                raise PedigreeError(f"predict_offspring: unknown individual {pid!r}")

    return p, e, individuals, topo, predict


def infer(spec):
    """Run exact inference.

    Returns a dict with per-individual posterior distributions, the evidence
    probability, and (optionally) the predicted offspring genotype
    distribution. Raises PedigreeError if the evidence probability is zero
    (contradictory observations).
    """
    p, e, individuals, topo, predict = _validate(spec)
    ids = topo
    index = {iid: k for k, iid in enumerate(ids)}

    # Founder priors under Hardy-Weinberg equilibrium.
    hw = (p * p, 2 * p * (1 - p), (1 - p) * (1 - p))

    # Observation likelihoods: P(observation | true genotype).
    def obs_likelihood(obs):
        if obs is None:
            return (1.0, 1.0, 1.0)
        return tuple(
            (1.0 - e) if g == obs else e / 2.0 for g in GENOTYPES
        )

    obs_lik = {iid: obs_likelihood(individuals[iid]["observation"]) for iid in ids}

    marginals = {iid: [0.0, 0.0, 0.0] for iid in ids}
    pair_joint = None
    if predict is not None:
        pa, pb = predict
        pair_joint = [[0.0] * 3 for _ in range(3)]

    evidence = 0.0
    for assignment in _iter_assignments(3, len(ids)):
        weight = 1.0
        for k, iid in enumerate(ids):
            g = assignment[k]
            parents = individuals[iid]["parents"]
            if parents:
                gp = assignment[index[parents[0]]]
                gm = assignment[index[parents[1]]]
                weight *= MENDEL[(gp, gm)][g]
            else:
                weight *= hw[g]
            weight *= obs_lik[iid][g]
            if weight == 0.0:
                break
        if weight == 0.0:
            continue
        evidence += weight
        for k, iid in enumerate(ids):
            marginals[iid][assignment[k]] += weight
        if predict is not None:
            ga = assignment[index[pa]]
            gb = assignment[index[pb]]
            pair_joint[ga][gb] += weight

    if evidence == 0.0:
        raise PedigreeError(
            "evidence probability is zero: observations are mutually "
            "contradictory under the model"
        )

    posterior = {
        iid: {g: marginals[iid][k] / evidence for k, g in enumerate(GENOTYPES)}
        for iid in ids
    }

    result = {
        "evidence_probability": evidence,
        "posterior": posterior,
    }

    if predict is not None:
        pa, pb = predict
        offspring = [0.0, 0.0, 0.0]
        for ga in range(3):
            for gb in range(3):
                joint = pair_joint[ga][gb] / evidence
                if joint == 0.0:
                    continue
                trans = MENDEL[(ga, gb)]
                for g in range(3):
                    offspring[g] += joint * trans[g]
        result["offspring_prediction"] = {
            "parents": [pa, pb],
            "distribution": {g: offspring[k] for k, g in enumerate(GENOTYPES)},
        }

    return result


def _iter_assignments(n_values, n_vars):
    """Yield tuples of range(n_values) of length n_vars (cartesian product)."""
    import itertools

    return itertools.product(range(n_values), repeat=n_vars)
