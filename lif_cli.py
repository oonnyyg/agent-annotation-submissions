"""Command line interface for the delayed LIF network simulator.

Usage:
    python lif_cli.py CONFIG.json [-o OUTPUT.json] [--voltages]

Prints a summary (spike counts, mean rates, undelivered spikes) and
optionally writes the full recordings (post-reset voltage table and
spike times) to a JSON file.
"""

from __future__ import annotations

import argparse
import json
import sys

from lif_config import ConfigError, load_config


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="lif_cli",
        description=(
            "Replay a leaky integrate-and-fire network with delayed "
            "synapses on a fixed time grid (no training)."
        ),
    )
    parser.add_argument("config", help="path to the JSON network configuration")
    parser.add_argument(
        "-o",
        "--output",
        help="write full recordings (voltages, spike times) to this JSON file",
    )
    parser.add_argument(
        "--voltages",
        action="store_true",
        help="also print the per-step post-reset voltage table to stdout",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        network = load_config(args.config)
    except (ConfigError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    result = network.run()
    duration_ms = result.num_steps * result.dt

    print(
        f"simulated {result.num_steps} steps of {result.dt} ms "
        f"({duration_ms} ms total), {len(result.neuron_names)} neuron(s)"
    )
    header = f"{'neuron':<12} {'spikes':>6} {'rate (Hz)':>10}  spike times (ms)"
    print(header)
    print("-" * len(header))
    for i, name in enumerate(result.neuron_names):
        times = ", ".join(f"{t:g}" for t in result.spike_times[i]) or "-"
        print(
            f"{name:<12} {result.spike_counts[i]:>6} "
            f"{result.mean_rates_hz[i]:>10.2f}  {times}"
        )
    print(f"undelivered spikes at end of simulation: {result.undelivered_spikes}")

    if args.voltages:
        print("\npost-reset voltages (mV) per step:")
        print("step\t" + "\t".join(result.neuron_names))
        for step, row in enumerate(result.voltages):
            cells = "\t".join(f"{v:.6g}" for v in row)
            print(f"{step}\t{cells}")

    if args.output:
        payload = {
            "dt": result.dt,
            "num_steps": result.num_steps,
            "neuron_names": result.neuron_names,
            "voltages": result.voltages,
            "spike_times": {
                name: result.spike_times[i]
                for i, name in enumerate(result.neuron_names)
            },
            "spike_counts": dict(zip(result.neuron_names, result.spike_counts)),
            "mean_rates_hz": dict(
                zip(result.neuron_names, result.mean_rates_hz)
            ),
            "undelivered_spikes": result.undelivered_spikes,
        }
        with open(args.output, "w", encoding="utf-8") as handle:
            json.dump(payload, handle, indent=2)
        print(f"recordings written to {args.output}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
