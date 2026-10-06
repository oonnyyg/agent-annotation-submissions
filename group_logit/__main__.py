"""Command line interface: python -m group_logit train|predict ..."""

import argparse
import sys

from .errors import GroupLogitError
from .predict import run_predict
from .train import run_train


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="group_logit",
        description="Grouped logistic regression training and prediction.")
    sub = parser.add_subparsers(dest="command", required=True)

    p_train = sub.add_parser("train", help="train with grouped CV and save a model")
    p_train.add_argument("--config", required=True, help="path to JSON config")

    p_pred = sub.add_parser("predict", help="predict a new CSV with a saved model")
    p_pred.add_argument("--model", required=True, help="path to model JSON")
    p_pred.add_argument("--csv", required=True, help="input feature CSV")
    p_pred.add_argument("--out", required=True, help="output predictions CSV")

    args = parser.parse_args(argv)
    try:
        if args.command == "train":
            run_train(args.config)
        else:
            run_predict(args.model, args.csv, args.out)
    except GroupLogitError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
