"""CLI: python -m pv_string input.json --csv-dir out --report report.json

Input JSON format:
{
  "step": 0.05,                  # CSV sampling step in amps (optional)
  "diode_drop": 0.5,             # default diode drop for all modules
  "conditions": [
    {
      "name": "uniform",
      "modules": [
        {"voc": 40, "isc": 10, "vknee": 32, "iknee": 9.5},
        ...
      ]
    }
  ]
}

Each module may override "diode_drop". Module order is preserved in the
report. No inverter or temperature behaviour is modelled.
"""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .model import Module
from .solver import analyze_condition, sweep


def _load_modules(spec: dict, default_drop: float) -> list[Module]:
    modules = []
    for entry in spec["modules"]:
        modules.append(
            Module(
                voc=float(entry["voc"]),
                isc=float(entry["isc"]),
                vknee=float(entry["vknee"]),
                iknee=float(entry["iknee"]),
                diode_drop=float(entry.get("diode_drop", default_drop)),
            )
        )
    if not modules:
        raise ValueError("each condition needs at least one module")
    return modules


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="pv_string",
        description="Global MPP of a shaded PV string (piecewise-linear model).",
    )
    parser.add_argument("input", help="JSON input file")
    parser.add_argument("--csv-dir", default=".", help="directory for sweep CSVs")
    parser.add_argument("--report", default="report.json", help="peak report JSON")
    args = parser.parse_args(argv)

    spec = json.loads(Path(args.input).read_text(encoding="utf-8"))
    step = float(spec.get("step", 0.05))
    default_drop = float(spec.get("diode_drop", 0.5))

    csv_dir = Path(args.csv_dir)
    csv_dir.mkdir(parents=True, exist_ok=True)

    conditions = []
    for cond in spec["conditions"]:
        name = cond.get("name", f"condition_{len(conditions)}")
        modules = _load_modules(cond, default_drop)

        csv_path = csv_dir / f"{name}.csv"
        with csv_path.open("w", newline="", encoding="utf-8") as fh:
            writer = csv.writer(fh)
            writer.writerow(["current_a", "voltage_v", "power_w"])
            writer.writerows(sweep(modules, step))

        result = analyze_condition(modules)
        result["name"] = name
        result["csv"] = str(csv_path)
        conditions.append(result)

    report = {"conditions": conditions}
    Path(args.report).write_text(
        json.dumps(report, indent=2) + "\n", encoding="utf-8"
    )
    for cond in conditions:
        bypassed = [m["index"] for m in cond["modules"] if m["bypassed"]]
        print(
            f"{cond['name']}: Pmpp={cond['power_w']:.3f} W "
            f"@ I={cond['current_a']:.4f} A, V={cond['voltage_v']:.3f} V; "
            f"bypassed modules: {bypassed or 'none'}"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
