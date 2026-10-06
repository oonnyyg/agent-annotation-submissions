"""Command line interface: python -m elastic_modes config.json [options]."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .core import InputError, Material, analyze


def load_config(path):
    try:
        with open(path, "r", encoding="utf-8") as fh:
            cfg = json.load(fh)
    except OSError as exc:
        raise InputError(f"cannot read config file: {exc}") from None
    except json.JSONDecodeError as exc:
        raise InputError(f"invalid JSON in config file: {exc}") from None
    if not isinstance(cfg, dict):
        raise InputError("config root must be a JSON object")
    return cfg


def build_material(cfg):
    if "material" in cfg:
        sub = cfg["material"]
        if not isinstance(sub, dict):
            raise InputError('"material" must be an object')
        merged = dict(cfg)
        merged.pop("material")
        merged.update(sub)
        cfg = merged
    return Material.from_config(cfg)


def get_directions(cfg):
    directions = cfg.get("directions")
    if not isinstance(directions, list) or not directions:
        raise InputError('"directions" must be a non-empty list of 3-vectors')
    return directions


def write_velocities_csv(path, results):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "direction_index",
                "n_x",
                "n_y",
                "n_z",
                "mode",
                "velocity_m_per_s",
                "degeneracy_group",
                "degeneracy_dimension",
                "polarization_x",
                "polarization_y",
                "polarization_z",
            ]
        )
        for res in results:
            for mode in res.modes:
                pol = mode.polarization or [None, None, None]
                writer.writerow(
                    [
                        res.index,
                        *[f"{v:.17g}" for v in res.direction],
                        mode.index,
                        f"{mode.velocity:.17g}",
                        mode.degeneracy_group,
                        mode.degeneracy_dimension,
                        *["" if v is None else f"{v:.17g}" for v in pol],
                    ]
                )


def write_diagnostics_json(path, results, rtol):
    doc = {"degeneracy_rtol": rtol, "directions": []}
    for res in results:
        entry = {
            "direction_index": res.index,
            "direction": res.direction,
            "orthogonality_error": res.orthogonality_error,
            "degeneracy_groups": [
                {
                    "group": gid,
                    "dimension": next(
                        m.degeneracy_dimension for m in res.modes if m.degeneracy_group == gid
                    ),
                    "projection_matrix": proj,
                }
                for gid, proj in sorted(res.projections.items())
            ],
            "modes": [
                {
                    "mode": m.index,
                    "eigenvalue_pa": m.eigenvalue,
                    "velocity_m_per_s": m.velocity,
                    "polarization": m.polarization,
                    "degeneracy_group": m.degeneracy_group,
                    "degeneracy_dimension": m.degeneracy_dimension,
                    "eigen_residual": m.residual,
                }
                for m in res.modes
            ],
        }
        doc["directions"].append(entry)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(doc, fh, indent=2)
        fh.write("\n")


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="python -m elastic_modes",
        description="Elastic wave speeds and polarization modes for anisotropic media.",
    )
    parser.add_argument("config", help="JSON configuration file")
    parser.add_argument("--velocities", default="velocities.csv", help="output CSV path")
    parser.add_argument("--diagnostics", default="diagnostics.json", help="output diagnostics JSON path")
    args = parser.parse_args(argv)

    try:
        cfg = load_config(args.config)
        material = build_material(cfg)
        directions = get_directions(cfg)
        rtol = cfg.get("degeneracy_rtol", 1e-6)
        results = analyze(material, directions, degeneracy_rtol=rtol)
        write_velocities_csv(args.velocities, results)
        write_diagnostics_json(args.diagnostics, results, rtol)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    print(f"wrote {args.velocities} and {args.diagnostics}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
