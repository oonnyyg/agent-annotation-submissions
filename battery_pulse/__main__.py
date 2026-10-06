"""CLI: python -m battery_pulse config.json --out outdir

Config JSON schema:
{
  "battery": {
    "capacity_ah": 2.5,
    "r0_ohm": 0.05,
    "ocv_grid": [{"soc": 0.0, "voltage": 3.0}, ..., {"soc": 1.0, "voltage": 4.2}],
    "rc_branches": [{"r_ohm": 0.02, "c_f": 2000.0}]
  },
  "cutoff_voltage_v": 2.5,
  "initial_soc": 1.0,
  "initial_v_pol_v": [0.0],
  "sample_dt_s": 1.0,
  "scenarios": {
    "pulse": [{"duration_s": 10, "current_a": 5.0},
              {"duration_s": 60, "current_a": 0.0}]
  }
}
"""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .model import ModelError, load_battery
from .sim import Segment, simulate


def _write_csv(path: Path, samples) -> None:
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["time_s", "side", "current_a", "soc", "v_oc_v",
                         "v_ohm_v", "v_pol_v", "v_terminal_v"])
        for s in samples:
            writer.writerow([f"{s.t:.9g}", s.side, f"{s.current_a:.9g}",
                             f"{s.soc:.9g}", f"{s.v_oc:.9g}", f"{s.v_ohm:.9g}",
                             f"{s.v_pol:.9g}", f"{s.v_term:.9g}"])


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="battery_pulse",
                                     description="Battery pulse load simulator")
    parser.add_argument("config", help="path to JSON configuration")
    parser.add_argument("--out", default="out", help="output directory")
    args = parser.parse_args(argv)

    try:
        cfg = json.loads(Path(args.config).read_text(encoding="utf-8"))
        battery = load_battery(cfg["battery"])
        v_min = float(cfg["cutoff_voltage_v"])
        soc0 = float(cfg.get("initial_soc", 1.0))
        v_pol0 = cfg.get("initial_v_pol_v")
        sample_dt = float(cfg.get("sample_dt_s", 1.0))
        scenarios = cfg["scenarios"]
        if not scenarios:
            raise ModelError("at least one scenario is required")

        outdir = Path(args.out)
        outdir.mkdir(parents=True, exist_ok=True)
        report = {}
        for name, segs in scenarios.items():
            segments = [Segment(float(s["duration_s"]), float(s["current_a"]))
                        for s in segs]
            samples, summary = simulate(battery, segments, v_min, soc0,
                                        v_pol0, sample_dt)
            _write_csv(outdir / f"{name}.csv", samples)
            report[name] = summary
        (outdir / "report.json").write_text(
            json.dumps(report, indent=2) + "\n", encoding="utf-8")
    except (ModelError, KeyError, TypeError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    for name, summary in report.items():
        print(f"{name}: {summary['stop_reason']} at t={summary['stop_time_s']:.6g}s, "
              f"net discharge {summary['net_discharge_ah']:.6g} Ah")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
