"""Pedigree genotype inference for a single biallelic locus (teaching tool)."""

from .core import (
    GENOTYPES,
    PedigreeError,
    InferenceResult,
    load_pedigree,
    infer,
    predict_offspring,
)

__all__ = [
    "GENOTYPES",
    "PedigreeError",
    "InferenceResult",
    "load_pedigree",
    "infer",
    "predict_offspring",
]
