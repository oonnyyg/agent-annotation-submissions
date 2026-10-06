"""CSV and JSON report writers."""
 
from __future__ import annotations
 
import csv
import json
from typing import TextIO
 
from .model import Network
from .solver import Solution
 
 
def write_csv(net: Network, sol: Solution, out: TextIO) -> None:
    writer = csv.writer(out)
    header = ["time"]
    for nuc in net.nuclides:
        header.append(f"quantity[{nuc.name}]")
    for nuc in net.nuclides:
        header.append(f"activity[{nuc.name}]")
    writer.writerow(header)
    for k, t in enumerate(sol.times):
        row = [f"{t:.17g}"]
        row += [f"{q:.17g}" for q in sol.quantities[k]]
        row += [f"{a:.17g}" for a in sol.activities[k]]
        writer.writerow(row)
 
 
def build_window_report(net: Network, sol: Solution) -> dict:
    windows = []
    for w in sol.windows:
        decays = {net.nuclides[i].name: w.decays[i] for i in range(net.size)}
        yields = {}
        for i, nuc in enumerate(net.nuclides):
            for (child, _), value in zip(net.branches[i], w.branch_yields[i]):
                yields.setdefault(nuc.name, {})[net.nuclides[child].name] = value
        windows.append({
            "start": w.start,
            "end": w.end,
            "decays": decays,
            "branch_yields": yields,
            "total_decays": sum(w.decays),
        })
    return {
        "windows": windows,
        "conservation": {
            "description": (
                "residual = total quantity - initial total - cumulative feed; "
                "decays conserve nuclei because each decay yields one child"),
            "initial_total": sum(nuc.initial for nuc in net.nuclides),
            "per_time": [
                {"time": t,
                 "cumulative_feed": f,
                 "residual": r}
                for t, f, r in zip(sol.times, sol.cumulative_feed,
                                   sol.conservation_residual)
            ],
            "max_abs_residual": max((abs(r) for r in sol.conservation_residual),
                                    default=0.0),
        },
        "precision": sol.precision,
    }
 
 
def write_json(net: Network, sol: Solution, out: TextIO) -> None:
    json.dump(build_window_report(net, sol), out, indent=2)
    out.write("\n")
