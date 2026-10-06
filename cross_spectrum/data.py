"""CSV loading: records are positioned by an equally spaced sample index."""

from __future__ import annotations

import csv


class DataError(ValueError):
    """Raised when the input CSV cannot be used."""


def load_samples(path: str, index_column: str, channel1_column: str,
                 channel2_column: str) -> dict[int, tuple[float | None, float | None]]:
    """Return {sample_index: (ch1, ch2)}; a missing/empty value is None."""
    samples: dict[int, tuple[float | None, float | None]] = {}
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        if reader.fieldnames is None:
            raise DataError("CSV file has no header row")
        for col in (index_column, channel1_column, channel2_column):
            if col not in reader.fieldnames:
                raise DataError(f"CSV is missing column {col!r}")
        for row in reader:
            raw_index = (row.get(index_column) or "").strip()
            if not raw_index:
                continue
            try:
                index = int(raw_index)
            except ValueError:
                raise DataError(f"invalid sample index: {raw_index!r}") from None
            samples[index] = (
                _parse_value(row.get(channel1_column)),
                _parse_value(row.get(channel2_column)),
            )
    if not samples:
        raise DataError("CSV contains no usable samples")
    return samples


def _parse_value(raw: str | None) -> float | None:
    if raw is None:
        return None
    text = raw.strip()
    if not text:
        return None
    try:
        return float(text)
    except ValueError:
        return None
