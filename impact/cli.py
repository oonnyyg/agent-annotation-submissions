"""Command line interface: JSON in, CSV trajectory + JSON diagnostics out."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .core import Disk, SimConfig, SimError, run


def load_input(path: str) -> tuple[list[Disk], SimConfig]:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    cfg = SimConfig(
        width=float(data["width"]),
        height=float(data["height"]),
        restitution=float(data.get("restitution", 1.0)),
        friction=float(data.get("friction", 0.0)),
        t_end=float(data["t_end"]),
        dt_sample=float(data["dt_sample"]),
        max_events=int(data.get("max_events", 10000)),
    )
    disks = [Disk(x=float(d["x"]), y=float(d["y"]),
                  vx=float(d.get("vx", 0.0)), vy=float(d.get("vy", 0.0)),
                  omega=float(d.get("omega", 0.0)),
                  mass=float(d.get("mass", 1.0)),
                  radius=float(d["radius"]))
             for d in data["disks"]]
    return disks, cfg


def write_csv(sim, path: str) -> None:
    with open(path, "w", newline="", encoding="utf-8") as fh:
        w = csv.writer(fh)
        w.writerow(["t", "disk", "x", "y", "vx", "vy", "omega"])
        for s in sim.samples:
            for k, d in enumerate(s["disks"]):
                w.writerow([f"{s['t']:.12g}", k,
                            f"{d['x']:.12g}", f"{d['y']:.12g}",
                            f"{d['vx']:.12g}", f"{d['vy']:.12g}",
                            f"{d['omega']:.12g}"])


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(
        prog="planar-impact",
        description="Event-driven 2D spinning-disk impact replay.")
    ap.add_argument("input", help="JSON scenario file")
    ap.add_argument("--csv", help="write trajectory samples to this CSV")
    ap.add_argument("--events", help="write event log to this JSON file")
    ap.add_argument("--diagnostics", help="write diagnostics JSON here")
    args = ap.parse_args(argv)

    try:
        disks, cfg = load_input(args.input)
        sim = run(disks, cfg)
    except SimError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    if args.csv:
        write_csv(sim, args.csv)
    if args.events:
        with open(args.events, "w", encoding="utf-8") as fh:
            json.dump(sim.events, fh, indent=2)
    diag = sim.diagnostics()
    text = json.dumps(diag, indent=2)
    if args.diagnostics:
        with open(args.diagnostics, "w", encoding="utf-8") as fh:
            fh.write(text + "\n")
    else:
        print(text)
    return 0 if sim.completed else 1


if __name__ == "__main__":
    raise SystemExit(main())
