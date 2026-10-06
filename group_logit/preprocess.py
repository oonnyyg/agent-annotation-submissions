"""Feature standardization fitted on training data only."""

from __future__ import annotations

import math

from .errors import UserError


class Standardizer:
    """Column-wise mean and population standard deviation.

    Constant columns get a scale of 1.0 so they remain constant after
    transformation instead of causing a division by zero.
    """

    def __init__(self, feature_names, means, scales):
        self.feature_names = list(feature_names)
        self.means = list(means)
        self.scales = list(scales)

    @classmethod
    def fit(cls, feature_names, rows):
        n = len(rows)
        if n == 0:
            raise UserError("cannot fit preprocessing on an empty training set")
        p = len(feature_names)
        means = [0.0] * p
        for row in rows:
            for j in range(p):
                means[j] += row[j]
        means = [m / n for m in means]
        variances = [0.0] * p
        for row in rows:
            for j in range(p):
                diff = row[j] - means[j]
                variances[j] += diff * diff
        scales = []
        for j in range(p):
            std = math.sqrt(variances[j] / n)
            scales.append(std if std > 0.0 else 1.0)
        return cls(feature_names, means, scales)

    def transform_row(self, row):
        return [(row[j] - self.means[j]) / self.scales[j] for j in range(len(self.means))]

    def transform(self, rows):
        return [self.transform_row(row) for row in rows]

    def to_dict(self):
        return {
            "feature_names": self.feature_names,
            "means": self.means,
            "scales": self.scales,
        }

    @classmethod
    def from_dict(cls, raw):
        try:
            names = raw["feature_names"]
            means = raw["means"]
            scales = raw["scales"]
        except (KeyError, TypeError):
            raise UserError("model file has an invalid preprocessing section") from None
        if not (isinstance(names, list) and isinstance(means, list) and isinstance(scales, list)):
            raise UserError("model file has an invalid preprocessing section")
        if not (len(names) == len(means) == len(scales)) or not names:
            raise UserError("model preprocessing arrays must be non-empty and equal length")
        if not all(isinstance(n, str) for n in names):
            raise UserError("model feature names must be strings")
        for value in list(means) + list(scales):
            if not isinstance(value, (int, float)) or not math.isfinite(value):
                raise UserError("model preprocessing values must be finite numbers")
        if any(s <= 0 for s in scales):
            raise UserError("model preprocessing scales must be positive")
        return cls(names, [float(m) for m in means], [float(s) for s in scales])
