"""生成示例数据 examples/isotherms.csv 与 examples/config.json。"""

import csv
import json
from pathlib import Path

HERE = Path(__file__).parent


def bet(ps, q_m, c):
    return [q_m * c * p / ((1 - p) * (1 - p + c * p)) for p in ps]


def main():
    ps = [0.01, 0.03, 0.05, 0.07, 0.10, 0.13, 0.16, 0.20, 0.25, 0.30]
    samples = {
        "carbon_a": bet(ps, 2.5e-3, 120.0),
        "silica_b": bet(ps, 4.0e-3, 80.0),
    }
    with (HERE / "isotherms.csv").open("w", newline="", encoding="utf-8") as fh:
        w = csv.writer(fh)
        w.writerow(["p_rel", *samples])
        for i, p in enumerate(ps):
            w.writerow([p, *[q[i] for q in samples.values()]])
    (HERE / "config.json").write_text(
        json.dumps({"cross_section_nm2": 0.162, "min_points": 4}, indent=2),
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
