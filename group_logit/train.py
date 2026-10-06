"""Grouped cross-validation, lambda selection and final refit."""

import csv
import json

from .data import load_config, load_training_csv
from .errors import GroupLogitError
from .optimize import _sigmoid, apply_scaler, fit_logistic, fit_scaler, log_loss


def _scores(features, w, b):
    return [b + sum(wj * xj for wj, xj in zip(w, row)) for row in features]


def run_train(config_path, report=print):
    cfg = load_config(config_path)
    feature_names, rows = load_training_csv(
        cfg["train_csv"], cfg["label_column"], cfg["group_column"], cfg["folds"])
    fold_ids = sorted({r["fold"] for r in rows})
    # Each fold's training part must contain both classes.
    for fold in fold_ids:
        train_labels = {r["label"] for r in rows if r["fold"] != fold}
        if train_labels != {0, 1}:
            raise GroupLogitError(
                f"training part for fold {fold} must contain both classes")
    labels_all = [r["label"] for r in rows]
    if set(labels_all) != {0, 1}:
        raise GroupLogitError("full training data must contain both classes")

    n = len(rows)
    results = []
    oof_prob = [None] * n
    for lam in cfg["lambdas"]:
        total_loss = 0.0
        iterations = 0
        for fold in fold_ids:
            train_rows = [r for r in rows if r["fold"] != fold]
            val_rows = [r for r in rows if r["fold"] == fold]
            means, scales = fit_scaler([r["features"] for r in train_rows])
            x_train = apply_scaler([r["features"] for r in train_rows], means, scales)
            y_train = [r["label"] for r in train_rows]
            w, b, info = fit_logistic(
                x_train, y_train, lam, cfg["max_iter"], cfg["tol"])
            iterations += info["iterations"]
            x_val = apply_scaler([r["features"] for r in val_rows], means, scales)
            scores = _scores(x_val, w, b)
            total_loss += log_loss(scores, [r["label"] for r in val_rows]) * len(val_rows)
            for r, z in zip(val_rows, scores):
                oof_prob[r["row"]] = _sigmoid(z)
        mean_loss = total_loss / n
        results.append({"lambda": lam, "oof_loss": mean_loss,
                        "iterations": iterations, "converged": True})
        report(f"lambda={lam:g}  oof_loss={mean_loss:.6f}  "
               f"iterations={iterations}  converged=yes")

    best = min(results, key=lambda r: (r["oof_loss"], -r["lambda"]))
    report(f"selected lambda={best['lambda']:g} (oof_loss={best['oof_loss']:.6f})")

    # Final refit on all data with preprocessing fit on all rows.
    means, scales = fit_scaler([r["features"] for r in rows])
    x_all = apply_scaler([r["features"] for r in rows], means, scales)
    w, b, info = fit_logistic(x_all, labels_all, best["lambda"],
                              cfg["max_iter"], cfg["tol"])
    report(f"final fit: iterations={info['iterations']} "
           f"objective={info['objective']:.6f} converged=yes")

    model = {
        "format": "group_logit-model",
        "version": 1,
        "feature_names": feature_names,
        "means": means,
        "scales": scales,
        "weights": w,
        "intercept": b,
        "lambda": best["lambda"],
        "candidates": results,
        "final_objective": info["objective"],
        "final_iterations": info["iterations"],
    }
    with open(cfg["model_out"], "w", encoding="utf-8") as fh:
        json.dump(model, fh, indent=2)
        fh.write("\n")

    with open(cfg["predictions_out"], "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["row", "group", "fold", "label", "probability"])
        for r in rows:
            writer.writerow([r["row"], r["group"], r["fold"], r["label"],
                             f"{oof_prob[r['row']]:.12g}"])
    return model
