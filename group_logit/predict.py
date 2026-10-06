"""Load a saved model and predict new CSV data (columns matched by name)."""

import csv
import json

from .data import load_predict_csv
from .errors import GroupLogitError
from .optimize import _sigmoid, apply_scaler


def load_model(path):
    with open(path, "r", encoding="utf-8") as fh:
        try:
            model = json.load(fh)
        except json.JSONDecodeError as exc:
            raise GroupLogitError(f"invalid model JSON: {exc}") from exc
    if not isinstance(model, dict) or model.get("format") != "group_logit-model":
        raise GroupLogitError("not a group_logit model file")
    for key in ("feature_names", "means", "scales", "weights", "intercept"):
        if key not in model:
            raise GroupLogitError(f"model file missing key: {key!r}")
    return model


def run_predict(model_path, csv_path, out_path):
    model = load_model(model_path)
    rows = load_predict_csv(csv_path, model["feature_names"])
    x = apply_scaler(rows, model["means"], model["scales"])
    w = model["weights"]
    b = model["intercept"]
    with open(out_path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["row", "probability", "prediction"])
        for index, features in enumerate(x):
            z = b + sum(wj * xj for wj, xj in zip(w, features))
            prob = _sigmoid(z)
            writer.writerow([index, f"{prob:.12g}", 1 if prob >= 0.5 else 0])
