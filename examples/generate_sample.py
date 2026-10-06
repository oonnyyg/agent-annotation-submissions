"""Generate a small synthetic sample dataset for the CLI demo."""

import csv
import json
import math
from pathlib import Path

RX = (1_000_000.0, -2_000_000.0, 5_000_000.0)
CLOCK = 12_345.0
SAT_POSITIONS = [
    (20_000_000.0, 5_000_000.0, 15_000_000.0),
    (-12_000_000.0, 18_000_000.0, 14_000_000.0),
    (8_000_000.0, -20_000_000.0, 13_000_000.0),
    (-15_000_000.0, -10_000_000.0, 20_000_000.0),
    (3_000_000.0, 12_000_000.0, 22_000_000.0),
]


def main() -> None:
    out = Path(__file__).resolve().parent
    with open(out / "observations.csv", "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["epoch", "sat_id", "x", "y", "z",
                         "pseudorange", "sigma"])
        for i, pos in enumerate(SAT_POSITIONS):
            pr = math.dist(RX, pos) + CLOCK
            writer.writerow(["E1", f"G{i + 1:02d}", *pos, f"{pr:.3f}", 1.0])
    (out / "initials.json").write_text(
        json.dumps({"E1": [0.0, 0.0, 0.0, 0.0]}, indent=2) + "\n",
        encoding="utf-8")
    print(f"wrote sample data to {out}")


if __name__ == "__main__":
    main()
