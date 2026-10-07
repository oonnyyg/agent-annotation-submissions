"""CLI: JSON scenario in, CSV trajectory and JSON diagnostics out."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .engine import (Disk, MultiContactError, SimConfig, SimError,
                     Simulation)


def load_scenario(path):
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    cfg = SimConfig(
        width=float(raw["width"]),
        height=float(raw["height"]),
        restitution=float(raw.get("restitution", 1.0)),
        friction=float(raw.get("friction", 0.0)),
        t_end=float(raw["t_end"]),
        sample_dt=float(raw["sample_dt"]),
        max_events=int(raw.get("max_events", 10000)),
    )
    disks = [
        Disk(x=float(d["x"]), y=float(d["y"]),
             vx=float(d.get("vx", 0.0)), vy=float(d.get("vy", 0.0)),
             omega=float(d.get("omega", 0.0)),
             mass=float(d["mass"]), radius=float(d["radius"]))
        for d in raw["disks"]
    ]
    return cfg, disks


def write_csv(sim, path):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["time", "disk", "x", "y", "vx", "vy", "omega"])
        for sample in sim.samples:
            for idx, state in enumerate(sample["disks"]):
                writer.writerow([
                    f"{sample['time']:.12g}", idx,
                    f"{state['x']:.12g}", f"{state['y']:.12g}",
                    f"{state['vx']:.12g}", f"{state['vy']:.12g}",
                    f"{state['omega']:.12g}",
                ])


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="planar-impact",
        description="Event-driven 2D rigid-disk impact replay.")
    parser.add_argument("scenario", help="JSON scenario file")
    parser.add_argument("--csv", help="write trajectory CSV here")
    parser.add_argument("--diagnostics", help="write diagnostics JSON here")
    args = parser.parse_args(argv)

    try:
        cfg, disks = load_scenario(args.scenario)
        sim = Simulation(cfg, disks).run()
    except MultiContactError as exc:
        print(json.dumps({"error": "multi-contact",
                          "contacts": exc.contacts}, indent=2),
              file=sys.stderr)
        return 2
    except (SimError, KeyError, ValueError) as exc:
        print(json.dumps({"error": str(exc)}), file=sys.stderr)
        return 1

    diag = sim.diagnostics()
    if args.csv:
        write_csv(sim, args.csv)
    text = json.dumps(diag, indent=2)
    if args.diagnostics:
        with open(args.diagnostics, "w", encoding="utf-8") as fh:
            fh.write(text + "\n")
    else:
        print(text)
    if not sim.completed:
        print("warning: event limit reached, simulation incomplete",
              file=sys.stderr)
        return 3
    return 0


if __name__ == "__main__":
    sys.exit(main())
