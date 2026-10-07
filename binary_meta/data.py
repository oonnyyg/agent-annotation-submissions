"""CSV loading and validation for study-level 2x2 count data."""

from __future__ import annotations

import csv
from dataclasses import dataclass


REQUIRED_COLUMNS = ("study_id", "e_events", "e_nonevents", "c_events", "c_nonevents")


class DataError(ValueError):
    """Raised for malformed input rows, with the CSV line number attached."""


@dataclass(frozen=True)
class Study:
    study_id: str
    e_events: int
    e_nonevents: int
    c_events: int
    c_nonevents: int

    @property
    def e_total(self) -> int:
        return self.e_events + self.e_nonevents

    @property
    def c_total(self) -> int:
        return self.c_events + self.c_nonevents


def _parse_count(raw: str, column: str, line_no: int, study_id: str) -> int:
    text = raw.strip()
    try:
        value = int(text)
    except ValueError:
        raise DataError(
            f"line {line_no} (study '{study_id}'): column '{column}' value "
            f"{text!r} is not an integer"
        ) from None
    if value < 0:
        raise DataError(
            f"line {line_no} (study '{study_id}'): column '{column}' value "
            f"{value} is negative"
        )
    return value


def load_studies(path: str) -> list[Study]:
    """Load and validate studies from a CSV file.

    Raises DataError with the offending line number for duplicate study ids,
    non-integer or negative counts, or non-positive group totals.
    """
    studies: list[Study] = []
    seen: set[str] = set()
    with open(path, newline="", encoding="utf-8-sig") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None:
            raise DataError("input file is empty")
        missing = [c for c in REQUIRED_COLUMNS if c not in reader.fieldnames]
        if missing:
            raise DataError(f"missing required columns: {', '.join(missing)}")
        for row_no, row in enumerate(reader, start=2):
            study_id = (row.get("study_id") or "").strip()
            if not study_id:
                raise DataError(f"line {row_no}: study_id is empty")
            if study_id in seen:
                raise DataError(f"line {row_no}: duplicate study id '{study_id}'")
            counts = {
                col: _parse_count(row.get(col) or "", col, row_no, study_id)
                for col in REQUIRED_COLUMNS[1:]
            }
            study = Study(study_id=study_id, **counts)
            if study.e_total <= 0:
                raise DataError(
                    f"line {row_no} (study '{study_id}'): experimental group total is zero"
                )
            if study.c_total <= 0:
                raise DataError(
                    f"line {row_no} (study '{study_id}'): control group total is zero"
                )
            seen.add(study_id)
            studies.append(study)
    if not studies:
        raise DataError("input file contains no study rows")
    return studies
