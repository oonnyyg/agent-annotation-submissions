"""Generate a synthetic example dataset and config for the CLI demo."""

import json
import math
import random


def bet_q(p, n_m, c):
    return n_m * c * p / ((1.0 - p) * (1.0 - p + c * p))


def main():
    rng = random.Random(42)
    rows = ["sample,relative_pressure,quantity_mol_per_g"]
    for name, n_m, c in [("zeolite-a", 3.0e-3, 120.0),
                         ("silica-b", 1.5e-3, 80.0)]:
        for i in range(14):
            p = 0.01 + 0.34 * i / 13
            q = bet_q(p, n_m, c) * (1.0 + rng.gauss(0.0, 0.002))
            rows.append(f"{name},{p:.5f},{q:.8e}")
    with open("examples/isotherms.csv", "w", encoding="utf-8") as fh:
        fh.write("\n".join(rows) + "\n")
    with open("examples/config.json", "w", encoding="utf-8") as fh:
        json.dump({"cross_section_nm2": 0.162, "min_points": 4}, fh, indent=2)


if __name__ == "__main__":
    main()
