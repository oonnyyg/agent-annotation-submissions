"""Loading a saved model and predicting new CSV data."""

from __future__ import annotations

import csv
import json
import math

from .errors import UserError
from .model import sigmoid
from .preprocess import Standardizer
from .train import MODEL_FORMAT_VERSION


def load_model(path):
    try:
        with open(path, "r", encoding="utf-8") as fh:
            raw = json.load(fh)
    except OSError as exc:
        raise UserError(f"cannot read model {path}: {exc}") from None
    except json.JSONDecodeError as exc:
        raise UserError(f"model {path} is not valid JSON: {exc}") from None
    if not isinstance(raw, dict):
        raise UserError("model file must be a JSON object")
    if raw.get("format_version") != MODEL_FORMAT_VERSION:
        raise UserError(f"unsupported model format version: {raw.get('format_version')!r}")
    standardizer = Standardizer.from_dict(raw.get("preprocessing"))
    intercept = raw.get("intercept")
    coefficients = raw.get("coefficients")
    if not isinstance(intercept, (int, float)) or not math.isfinite(intercept):
        raise UserError("model intercept must be a finite number")
    if not isinstance(coefficients, list) or len(coefficients) != len(standardizer.means):
        raise UserError("model coefficients must match the preprocessing feature count")
    for c in coefficients:
        if not isinstance(c, (int, float)) or not math.isfinite(c):
            raise UserError("model coefficients must be finite numbers")
    return standardizer, float(intercept), [float(c) for c in coefficients]


def run_predict(model_path, data_path, output_path):
    standardizer, intercept, coefficients = load_model(model_path)
    try:
        with open(data_path, "r", encoding="utf-8", newline="") as fh:
            rows = list(csv.reader(fh))
    except OSError as exc:
        raise UserError(f"cannot read data {data_path}: {exc}") from None
    if not rows:
        raise UserError(f"data {data_path} is empty")
    header = rows[0]
    if len(header) != len(set(header)):
        raise UserError("CSV header contains duplicate column names")
    missing = [name for name in standardizer.feature_names if name not in header]
    if missing:
        raise UserError(f"data is missing required feature columns: {missing}")
    index = {name: header.index(name) for name in standardizer.feature_names}

    output_rows = []
    for row_number, row in enumerate(rows[1:], start=2):
        if not row or all(cell.strip() == "" for cell in row):
            continue
        if len(row) != len(header):
            raise UserError(f"row {row_number}: expected {len(header)} columns, got {len(row)}")
        values = []
        for name in standardizer.feature_names:
            cell = row[index[name]].strip()
            if cell == "":
                raise UserError(f"row {row_number}: feature {name!r} is missing")
            try:
                value = float(cell)
            except ValueError:
                raise UserError(f"row {row_number}: feature {name!r} value {cell!r} is not a number") from None
            if not math.isfinite(value):
                raise UserError(f"row {row_number}: feature {name!r} is not finite")
            values.append(value)
        scaled = standardizer.transform_row(values)
        score = intercept + sum(c * x for c, x in zip(coefficients, scaled))
        prob = sigmoid(score)
        output_rows.append((row_number - 2, prob, int(prob >= 0.5)))

    try:
        with open(output_path, "w", encoding="utf-8", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(["row", "probability", "prediction"])
            for row_id, prob, label in output_rows:
                writer.writerow([row_id, repr(prob), label])
    except OSError as exc:
        raise UserError(f"cannot write predictions {output_path}: {exc}") from None
