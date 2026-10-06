"""CLI: python -m pv_string input.json [-o report.json] [--csv scan.csv]."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .model import Module, analyze_string


def _build_modules(specs, diode_drop):
    return [
        Module(
            voc=float(s["voc"]),
            isc=float(s["isc"]),
            knee_v=float(s["knee_v"]),
            knee_i=float(s["knee_i"]),
            diode_drop=diode_drop,
        )
        for s in specs
    ]


def _point_json(point):
    return {
        "current": point.current,
        "voltage": point.voltage,
        "power": point.power,
        "modules": [
            {"index": i, "voltage": s.voltage, "bypassed": s.bypassed}
            for i, s in enumerate(point.modules)
        ],
    }


def run(input_path, report_path, csv_path):
    with open(input_path, "r", encoding="utf-8") as fh:
        config = json.load(fh)

    diode_drop = float(config.get("diode_drop", 0.5))
    step = float(config.get("step", 0.1))
    if step <= 0.0:
        raise ValueError("step must be positive")
    default_modules = config["modules"]
    conditions = config.get("conditions") or [{"name": "default"}]

    report = {"diode_drop": diode_drop, "step": step, "conditions": []}
    csv_rows = []

    for condition in conditions:
        name = condition.get("name", "unnamed")
        specs = condition.get("modules", default_modules)
        modules = _build_modules(specs, diode_drop)
        analysis = analyze_string(modules)

        report["conditions"].append(
            {
                "name": name,
                "module_count": len(modules),
                "max_current": analysis.max_current,
                "optimum": _point_json(analysis.optimum),
                "local_peaks": [_point_json(p) for p in analysis.local_peaks],
            }
        )

        if csv_path is not None:
            n_steps = int(analysis.max_current / step) + 1
            for k in range(n_steps + 1):
                current = k * step
                voltages = [m.voltage(current) for m in modules]
                total = sum(voltages)
                row = {
                    "condition": name,
                    "current": current,
                    "total_voltage": total,
                    "power": current * total if total >= 0.0 else 0.0,
                }
                for i, v in enumerate(voltages):
                    row[f"module_{i}_voltage"] = v
                csv_rows.append(row)

    with open(report_path, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)

    if csv_path is not None:
        n_modules = report["conditions"][0]["module_count"]
        fieldnames = ["condition", "current", "total_voltage", "power"] + [
            f"module_{i}_voltage" for i in range(n_modules)
        ]
        with open(csv_path, "w", encoding="utf-8", newline="") as fh:
            writer = csv.DictWriter(fh, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(csv_rows)

    return report


def main(argv=None):
    parser = argparse.ArgumentParser(prog="pv_string")
    parser.add_argument("input", help="input JSON file")
    parser.add_argument("-o", "--report", default="report.json", help="peak report JSON")
    parser.add_argument("--csv", default=None, help="current sweep CSV output")
    args = parser.parse_args(argv)

    report = run(args.input, args.report, args.csv)
    for condition in report["conditions"]:
        opt = condition["optimum"]
        bypassed = [m["index"] for m in opt["modules"] if m["bypassed"]]
        print(
            f"[{condition['name']}] MPP: {opt['power']:.3f} W "
            f"@ {opt['current']:.4f} A, {opt['voltage']:.3f} V; "
            f"bypassed modules: {bypassed or 'none'}"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
