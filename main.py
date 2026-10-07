"""Command-line interface for the parallel-beam FBP reconstruction tool."""

from __future__ import annotations

import argparse
import sys

import fbp


def build_parser():
    parser = argparse.ArgumentParser(
        prog="parallel-beam-reconstruction",
        description=(
            "Reconstruct a 2D image from synthetic parallel-beam "
            "projections using filtered backprojection (standard "
            "library only; not for clinical imaging)."
        ),
    )
    parser.add_argument(
        "--projections",
        required=True,
        help="CSV file: one row per angle, first column angle in degrees",
    )
    parser.add_argument(
        "--config",
        required=True,
        help="JSON file with detector_spacing and image grid settings",
    )
    parser.add_argument(
        "--output",
        required=True,
        help="output CSV path for the reconstructed matrix",
    )
    parser.add_argument(
        "--summary",
        required=True,
        help="output JSON path for the reconstruction summary",
    )
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        config = fbp.load_config(args.config)
        angles, sinogram = fbp.load_sinogram(args.projections)
        image, summary = fbp.reconstruct(angles, sinogram, config)
        fbp.write_matrix_csv(args.output, image)
        fbp.write_summary_json(args.summary, summary)
    except fbp.ReconstructionError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except OSError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(
        f"reconstructed {summary['image']['nx']}x{summary['image']['ny']} "
        f"image from {summary['angles']['count']} angles x "
        f"{summary['detector']['count']} detectors"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
