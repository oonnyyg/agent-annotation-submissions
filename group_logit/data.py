"""CSV loading and strict validation for grouped logistic training."""

import csv
import json
import math

from .errors import GroupLogitError


def load_config(path):
    with open(path, "r", encoding="utf-8") as fh:
        try:
            cfg = json.load(fh)
        except json.JSONDecodeError as exc:
            raise GroupLogitError(f"invalid config JSON: {exc}") from exc
    if not isinstance(cfg, dict):
        raise GroupLogitError("config must be a JSON object")
    required = ["train_csv", "label_column", "group_column", "folds",
                "lambdas", "model_out", "predictions_out"]
    for key in required:
        if key not in cfg:
            raise GroupLogitError(f"config missing required key: {key!r}")
    if not isinstance(cfg["folds"], dict) or not cfg["folds"]:
        raise GroupLogitError("config 'folds' must be a non-empty object mapping group -> fold")
    folds = {}
    for group, fold in cfg["folds"].items():
        if not isinstance(fold, int) or isinstance(fold, bool) or fold < 0:
            raise GroupLogitError(f"fold for group {group!r} must be a non-negative integer")
        folds[str(group)] = fold
    lambdas = cfg["lambdas"]
    if not isinstance(lambdas, list) or not lambdas:
        raise GroupLogitError("config 'lambdas' must be a non-empty list")
    parsed = []
    for lam in lambdas:
        if not isinstance(lam, (int, float)) or isinstance(lam, bool) \
                or not math.isfinite(lam) or lam <= 0.0:
            raise GroupLogitError(f"lambda candidate must be a positive finite number, got {lam!r}")
        parsed.append(float(lam))
    max_iter = int(cfg.get("max_iter", 200))
    tol = float(cfg.get("tol", 1e-8))
    if max_iter <= 0 or tol <= 0.0:
        raise GroupLogitError("max_iter and tol must be positive")
    return {
        "train_csv": cfg["train_csv"],
        "label_column": cfg["label_column"],
        "group_column": cfg["group_column"],
        "folds": folds,
        "lambdas": parsed,
        "model_out": cfg["model_out"],
        "predictions_out": cfg["predictions_out"],
        "max_iter": max_iter,
        "tol": tol,
    }


def load_training_csv(path, label_column, group_column, folds):
    """Return (feature_names, rows) where rows are dicts with keys
    features/label/group/fold/row. Rejects any invalid row."""
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        header = reader.fieldnames or []
        if label_column not in header:
            raise GroupLogitError(f"label column {label_column!r} not found in CSV")
        if group_column not in header:
            raise GroupLogitError(f"group column {group_column!r} not found in CSV")
        feature_names = [c for c in header if c not in (label_column, group_column)]
        if not feature_names:
            raise GroupLogitError("CSV has no feature columns")
        rows = []
        for index, raw in enumerate(reader):
            row_no = index + 2  # 1-based, header is line 1
            label_text = (raw.get(label_column) or "").strip()
            if label_text not in ("0", "1"):
                raise GroupLogitError(
                    f"row {row_no}: label must be 0 or 1, got {label_text!r}")
            group = (raw.get(group_column) or "").strip()
            if not group:
                raise GroupLogitError(f"row {row_no}: missing group identifier")
            if group not in folds:
                raise GroupLogitError(f"row {row_no}: unknown group {group!r}")
            features = []
            for name in feature_names:
                text = (raw.get(name) or "").strip()
                if not text:
                    raise GroupLogitError(f"row {row_no}: missing feature {name!r}")
                try:
                    value = float(text)
                except ValueError:
                    raise GroupLogitError(
                        f"row {row_no}: feature {name!r} is not numeric: {text!r}")
                if not math.isfinite(value):
                    raise GroupLogitError(
                        f"row {row_no}: feature {name!r} is not finite: {text!r}")
                features.append(value)
            rows.append({
                "features": features,
                "label": int(label_text),
                "group": group,
                "fold": folds[group],
                "row": index,
            })
    if not rows:
        raise GroupLogitError("training CSV contains no data rows")
    return feature_names, rows


def load_predict_csv(path, feature_names):
    """Load a CSV for prediction; columns matched by name, order irrelevant."""
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        header = reader.fieldnames or []
        missing = [c for c in feature_names if c not in header]
        if missing:
            raise GroupLogitError(f"prediction CSV missing feature columns: {missing}")
        rows = []
        for index, raw in enumerate(reader):
            row_no = index + 2
            features = []
            for name in feature_names:
                text = (raw.get(name) or "").strip()
                if not text:
                    raise GroupLogitError(f"row {row_no}: missing feature {name!r}")
                try:
                    value = float(text)
                except ValueError:
                    raise GroupLogitError(
                        f"row {row_no}: feature {name!r} is not numeric: {text!r}")
                if not math.isfinite(value):
                    raise GroupLogitError(
                        f"row {row_no}: feature {name!r} is not finite: {text!r}")
                features.append(value)
            rows.append(features)
    if not rows:
        raise GroupLogitError("prediction CSV contains no data rows")
    return rows
