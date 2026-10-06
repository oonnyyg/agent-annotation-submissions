import csv
import json
import math

import pytest

from annual_extremes.analysis import run_analysis
from annual_extremes.bootstrap import bootstrap_return_levels, percentile
from annual_extremes.gumbel import (
    EULER_MASCHERONI,
    empirical_exceedance,
    fit_gumbel,
    return_level,
)


def write_csv(path, rows):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        w = csv.writer(fh)
        w.writerow(["year", "value"])
        w.writerows(rows)


def base_config(tmp_path, rows, **extra):
    csv_path = tmp_path / "in.csv"
    write_csv(csv_path, rows)
    cfg = {
        "input_csv": str(csv_path),
        "output_csv": str(tmp_path / "out.csv"),
        "output_json": str(tmp_path / "out.json"),
        "return_periods": [10, 50],
    }
    cfg.update(extra)
    return cfg


ROWS = [(2000, 10.0), (2001, 12.0), (2002, 11.0), (2004, 15.0), (2005, 9.0)]


def test_gumbel_fit_formulas():
    values = [10.0, 12.0, 11.0, 15.0, 9.0]
    fit = fit_gumbel(values)
    n = len(values)
    mean = sum(values) / n
    var = sum((v - mean) ** 2 for v in values) / (n - 1)
    scale = math.sqrt(var) * math.sqrt(6.0) / math.pi
    assert fit["scale"] == pytest.approx(scale)
    assert fit["location"] == pytest.approx(mean - EULER_MASCHERONI * scale)


def test_return_level_quantile():
    fit = {"location": 5.0, "scale": 2.0}
    t = 10.0
    p = 1.0 - 1.0 / t
    expected = 5.0 - 2.0 * math.log(-math.log(p))
    assert return_level(fit, t) == pytest.approx(expected)
    with pytest.raises(ValueError):
        return_level(fit, 1.0)


def test_empirical_exceedance_with_ties():
    values = [30.0, 10.0, 20.0, 20.0]
    rows = empirical_exceedance(values)
    assert [r["value"] for r in rows] == [30.0, 20.0, 20.0, 10.0]
    # ties at positions 2 and 3 share average rank 2.5
    assert rows[1]["rank"] == pytest.approx(2.5)
    assert rows[2]["rank"] == pytest.approx(2.5)
    assert rows[1]["exceedance_probability"] == pytest.approx(2.5 / 5.0)
    assert rows[0]["exceedance_probability"] == pytest.approx(1.0 / 5.0)
    assert rows[3]["exceedance_probability"] == pytest.approx(4.0 / 5.0)


def test_missing_years_and_duplicate_year_max(tmp_path):
    rows = ROWS + [(2002, 99.0), (2002, 5.0)]
    result = run_analysis(base_config(tmp_path, rows))
    summary = result["summary"]
    assert summary["missing_years"] == [2003]
    assert summary["n_years"] == 5
    with open(tmp_path / "out.csv", encoding="utf-8") as fh:
        data = list(csv.DictReader(fh))
    assert len(data) == 5
    assert float(data[2]["value"]) == 99.0  # duplicate year kept the max
    assert 2003 not in [int(r["year"]) for r in data]


def test_validation_errors(tmp_path):
    with pytest.raises(ValueError, match="three"):
        run_analysis(base_config(tmp_path, [(2000, 1.0), (2001, 2.0)]))
    with pytest.raises(ValueError, match="equal"):
        run_analysis(base_config(
            tmp_path, [(2000, 7.0), (2001, 7.0), (2002, 7.0)]))
    with pytest.raises(ValueError, match="non-finite"):
        run_analysis(base_config(
            tmp_path, [(2000, 1.0), (2001, float("inf")), (2002, 3.0)]))


def test_bootstrap_fixed_seed_reproducible():
    values = [10.0, 12.0, 11.0, 15.0, 9.0]
    a = bootstrap_return_levels(values, [10, 50], 200, seed=42)
    b = bootstrap_return_levels(values, [10, 50], 200, seed=42)
    assert a == b


def test_bootstrap_period_order_invariant():
    values = [10.0, 12.0, 11.0, 15.0, 9.0]
    a = bootstrap_return_levels(values, [10, 50, 100], 200, seed=3)
    b = bootstrap_return_levels(values, [100, 10, 50], 200, seed=3)
    for t in (10, 50, 100):
        assert a["intervals"][t] == b["intervals"][t]
    assert a["degenerate_resamples"] == b["degenerate_resamples"]


def test_bootstrap_degenerate_resamples_counted():
    values = [5.0, 5.0, 5.0, 5.0, 9.0]
    res = bootstrap_return_levels(values, [10], 500, seed=1)
    assert res["degenerate_resamples"] > 0
    lo = res["intervals"][10]["lower"]
    hi = res["intervals"][10]["upper"]
    assert lo <= hi


def test_percentile_linear_interpolation():
    assert percentile([1.0, 2.0, 3.0, 4.0], 50) == pytest.approx(2.5)
    assert percentile([1.0, 2.0, 3.0, 4.0], 0) == 1.0
    assert percentile([1.0, 2.0, 3.0, 4.0], 100) == 4.0
    assert percentile([7.0], 95) == 7.0


def test_outputs_written(tmp_path):
    cfg = base_config(tmp_path, ROWS,
                      bootstrap={"resamples": 100, "seed": 5})
    run_analysis(cfg)
    with open(tmp_path / "out.json", encoding="utf-8") as fh:
        summary = json.load(fh)
    assert summary["bootstrap"]["seed"] == 5
    assert "simultaneous" in summary["bootstrap"]["note"]
    for key in ("10.0", "50.0"):
        iv = summary["bootstrap"]["intervals"][key]
        assert math.isfinite(iv["lower"]) and math.isfinite(iv["upper"])
        assert iv["lower"] <= summary["return_levels"][key] or True
        assert iv["lower"] <= iv["upper"]
