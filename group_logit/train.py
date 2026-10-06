"""Training orchestration: grouped CV, lambda selection, final refit."""

from __future__ import annotations

import csv
import json

from .data import load_config, load_training_csv
from .errors import UserError
from .model import average_log_loss, fit_logistic, scores_for, sigmoid
from .preprocess import Standardizer

MODEL_FORMAT_VERSION = 1


def _split_folds(dataset, config):
    folds = {}
    for i, group in enumerate(dataset.groups):
        folds.setdefault(config.fold_assignments[group], []).append(i)
    return folds


def _check_train_classes(labels, indices, context):
    if {labels[i] for i in indices} != {0, 1}:
        raise UserError(f"{context}: training split must contain both classes 0 and 1")


def _cross_validate(dataset, config, lam):
    folds = _split_folds(dataset, config)
    oof_scores = [None] * len(dataset.labels)
    folds_status = []
    for fold_id in sorted(folds):
        val_idx = folds[fold_id]
        train_idx = [i for f, idxs in folds.items() if f != fold_id for i in idxs]
        _check_train_classes(dataset.labels, train_idx, f"lambda={lam} fold={fold_id}")
        standardizer = Standardizer.fit(
            dataset.feature_names, [dataset.features[i] for i in train_idx]
        )
        train_rows = standardizer.transform([dataset.features[i] for i in train_idx])
        fit = fit_logistic(
            train_rows,
            [dataset.labels[i] for i in train_idx],
            lam,
            config.max_iterations,
            config.tolerance,
        )
        folds_status.append(
            {
                "fold": fold_id,
                "iterations": fit.iterations,
                "converged": fit.converged,
                "objective": fit.final_objective,
            }
        )
        if not fit.converged:
            raise UserError(
                f"fitting did not converge for lambda={lam} on fold {fold_id} "
                f"after {fit.iterations} iterations; no model was saved"
            )
        val_rows = standardizer.transform([dataset.features[i] for i in val_idx])
        for i, score in zip(val_idx, scores_for(val_rows, fit.coefficients, fit.intercept)):
            oof_scores[i] = score
    if any(s is None for s in oof_scores):
        raise UserError("internal error: some rows did not receive an out-of-fold prediction")
    return oof_scores, folds_status


def _write_predictions(path, dataset, oof_scores):
    try:
        with open(path, "w", encoding="utf-8", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(["row", "group", "label", "probability", "prediction"])
            for i, score in enumerate(oof_scores):
                prob = sigmoid(score)
                writer.writerow(
                    [i, dataset.groups[i], dataset.labels[i], repr(prob), int(prob >= 0.5)]
                )
    except OSError as exc:
        raise UserError(f"cannot write predictions {path}: {exc}") from None


def run_train(data_path, config_path, predictions_path, model_path, report_path):
    config = load_config(config_path)
    dataset = load_training_csv(data_path, config)

    report = {"candidates": [], "selected_lambda": None}
    scores_by_lambda = {}
    candidate_losses = {}
    for lam in config.lambdas:
        oof_scores, folds_status = _cross_validate(dataset, config, lam)
        loss = average_log_loss(dataset.labels, oof_scores)
        candidate_losses[lam] = loss
        scores_by_lambda[lam] = oof_scores
        report["candidates"].append(
            {"lambda": lam, "oof_log_loss": loss, "folds": folds_status}
        )

    best_loss = min(candidate_losses.values())
    selected = max(lam for lam, loss in candidate_losses.items() if loss == best_loss)
    report["selected_lambda"] = selected
    report["selection_rule"] = (
        "minimum out-of-fold mean log loss over all samples; ties broken by larger lambda"
    )
    _write_predictions(predictions_path, dataset, scores_by_lambda[selected])

    standardizer = Standardizer.fit(dataset.feature_names, dataset.features)
    final_fit = fit_logistic(
        standardizer.transform(dataset.features),
        dataset.labels,
        selected,
        config.max_iterations,
        config.tolerance,
    )
    report["final_fit"] = {
        "lambda": selected,
        "iterations": final_fit.iterations,
        "converged": final_fit.converged,
        "objective": final_fit.final_objective,
    }
    if not final_fit.converged:
        raise UserError(
            f"final fit did not converge after {final_fit.iterations} iterations; "
            "no model was saved"
        )

    model = {
        "format_version": MODEL_FORMAT_VERSION,
        "lambda": selected,
        "intercept": final_fit.intercept,
        "coefficients": final_fit.coefficients,
        "preprocessing": standardizer.to_dict(),
    }
    try:
        with open(model_path, "w", encoding="utf-8") as fh:
            json.dump(model, fh, indent=2)
            fh.write("\n")
    except OSError as exc:
        raise UserError(f"cannot write model {model_path}: {exc}") from None

    if report_path:
        try:
            with open(report_path, "w", encoding="utf-8") as fh:
                json.dump(report, fh, indent=2)
                fh.write("\n")
        except OSError as exc:
            raise UserError(f"cannot write report {report_path}: {exc}") from None
