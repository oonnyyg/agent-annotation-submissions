"""Command line interface: python -m group_logit <command> [options]."""

from __future__ import annotations

import argparse
import sys

from .errors import UserError
from .predict import run_predict
from .train import run_train


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python -m group_logit",
        description=(
            "Train a binary logistic regression with group-aware "
            "cross-validation, or load a saved model to predict new data."
        ),
    )
    sub = parser.add_subparsers(dest="command", required=True)

    train = sub.add_parser("train", help="train a model and write validation predictions")
    train.add_argument("--data", required=True, help="training CSV with features, label and group columns")
    train.add_argument("--config", required=True, help="JSON training configuration")
    train.add_argument("--predictions", required=True, help="output CSV of out-of-fold validation predictions")
    train.add_argument("--model", required=True, help="output model JSON path")
    train.add_argument("--report", default=None, help="optional JSON report of candidate losses and convergence")

    predict = sub.add_parser("predict", help="load a saved model and predict a new CSV")
    predict.add_argument("--model", required=True, help="model JSON produced by train")
    predict.add_argument("--data", required=True, help="new CSV with feature columns (any order)")
    predict.add_argument("--output", required=True, help="output CSV with probabilities and predicted labels")

    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        if args.command == "train":
            run_train(args.data, args.config, args.predictions, args.model, args.report)
        else:
            run_predict(args.model, args.data, args.output)
    except UserError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
