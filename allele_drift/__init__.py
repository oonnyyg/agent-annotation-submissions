"""Finite haploid population allele drift calculator.

Model: a haploid population of fixed size N carries two types, "target"
and "normal". Each generation first undergoes selection (target relative
fitness is 1 + s, normal is 1), then bidirectional mutation (target ->
normal with probability v, normal -> target with probability u). The
next generation's target count is Binomial(N, p) where p is the
post-selection, post-mutation probability. The exact discrete
distribution is propagated with the full transition matrix; no random
simulation is used.
"""

from .core import Scenario, run_scenario, validate_config

__all__ = ["Scenario", "run_scenario", "validate_config"]
