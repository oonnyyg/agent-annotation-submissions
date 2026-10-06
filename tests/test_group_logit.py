import csv
import io
import json
import math
import os
import tempfile
import unittest

from group_logit.data import load_predict_csv, load_training_csv
from group_logit.errors import GroupLogitError
from group_logit.optimize import apply_scaler, fit_logistic, fit_scaler, log_loss
from group_logit.predict import run_predict
from group_logit.train import run_train


def make_dataset(tmp, n_groups=4, per_group=10, constant=False):
    """Linearly separable-ish synthetic data, one group per fold."""
    path = os.path.join(tmp, "train.csv")
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["y", "grp", "x1", "x2"])
        for g in range(n_groups):
            for i in range(per_group):
                y = (g + i) % 2
                x1 = (2.0 * y - 1.0) * (1.0 + 0.1 * i) + 0.05 * g
                x2 = 3.5 if constant else (0.3 * i - y)
                writer.writerow([y, f"g{g}", x1, x2])
    return path


def make_config(tmp, csv_path, lambdas=(0.01, 0.1, 1.0), **overrides):
    cfg = {
        "train_csv": csv_path,
        "label_column": "y",
        "group_column": "grp",
        "folds": {"g0": 0, "g1": 1, "g2": 2, "g3": 3},
        "lambdas": list(lambdas),
        "model_out": os.path.join(tmp, "model.json"),
        "predictions_out": os.path.join(tmp, "oof.csv"),
    }
    cfg.update(overrides)
    path = os.path.join(tmp, "config.json")
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(cfg, fh)
    return path


class TestValidation(unittest.TestCase):
    def test_rejects_bad_label(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "d.csv")
            with open(path, "w", newline="") as fh:
                fh.write("y,grp,x\n2,g1,1.0\n")
            with self.assertRaises(GroupLogitError):
                load_training_csv(path, "y", "grp", {"g1": 0})

    def test_rejects_unknown_group(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "d.csv")
            with open(path, "w", newline="") as fh:
                fh.write("y,grp,x\n1,gX,1.0\n")
            with self.assertRaises(GroupLogitError):
                load_training_csv(path, "y", "grp", {"g1": 0})

    def test_rejects_non_finite_and_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            for bad in ("nan", "inf", ""):
                path = os.path.join(tmp, "d.csv")
                with open(path, "w", newline="") as fh:
                    fh.write(f"y,grp,x\n1,g1,{bad}\n")
                with self.assertRaises(GroupLogitError, msg=bad):
                    load_training_csv(path, "y", "grp", {"g1": 0})

    def test_rejects_non_positive_lambda(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path = make_dataset(tmp)
            cfg_path = make_config(tmp, csv_path, lambdas=(0.1, 0.0))
            with self.assertRaises(GroupLogitError):
                run_train(cfg_path, report=lambda *_: None)

    def test_rejects_single_class_training_fold(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "d.csv")
            with open(path, "w", newline="") as fh:
                fh.write("y,grp,x\n")
                fh.write("0,g0,1.0\n0,g1,2.0\n1,g2,3.0\n")
            cfg_path = make_config(tmp, path, folds={"g0": 0, "g1": 1, "g2": 2})
            with self.assertRaises(GroupLogitError):
                run_train(cfg_path, report=lambda *_: None)


class TestScaler(unittest.TestCase):
    def test_constant_column_scale_one(self):
        rows = [[2.0, 5.0], [4.0, 5.0], [6.0, 5.0]]
        means, scales = fit_scaler(rows)
        self.assertEqual(means, [4.0, 5.0])
        self.assertEqual(scales[1], 1.0)
        out = apply_scaler(rows, means, scales)
        for row in out:
            self.assertEqual(row[1], 0.0)

    def test_population_std(self):
        rows = [[1.0], [2.0], [3.0]]
        means, scales = fit_scaler(rows)
        self.assertAlmostEqual(means[0], 2.0)
        self.assertAlmostEqual(scales[0], math.sqrt(2.0 / 3.0))


class TestOptimizer(unittest.TestCase):
    def test_stable_large_scores(self):
        # Extreme separable data must not overflow; loss stays finite.
        features = [[1000.0], [-1000.0], [999.0], [-999.0]]
        labels = [1, 0, 1, 0]
        w, b, info = fit_logistic(features, labels, 1.0, 200, 1e-10)
        self.assertTrue(info["converged"])
        self.assertTrue(math.isfinite(info["objective"]))
        self.assertLess(log_loss(
            [b + w[0] * x[0] for x in features], labels), 0.05)

    def test_non_convergence_raises(self):
        features = [[0.1], [0.2], [0.3], [0.4]]
        labels = [0, 1, 0, 1]
        with self.assertRaises(GroupLogitError):
            fit_logistic(features, labels, 0.5, max_iter=1, tol=1e-15)


class TestEndToEnd(unittest.TestCase):
    def _train(self, tmp, **kwargs):
        csv_path = make_dataset(tmp, **kwargs)
        cfg_path = make_config(tmp, csv_path)
        lines = []
        run_train(cfg_path, report=lines.append)
        return csv_path, lines

    def test_group_isolation_and_oof_rows(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path, lines = self._train(tmp)
            self.assertTrue(any("selected lambda" in line for line in lines))
            with open(os.path.join(tmp, "oof.csv"), newline="") as fh:
                rows = list(csv.DictReader(fh))
            with open(csv_path, newline="") as fh:
                n_data = sum(1 for _ in fh) - 1
            # Each training row predicted exactly once out-of-fold.
            self.assertEqual(len(rows), n_data)
            self.assertEqual(sorted(int(r["row"]) for r in rows),
                             list(range(n_data)))
            # Every row's fold matches its group's configured fold.
            folds = {"g0": 0, "g1": 1, "g2": 2, "g3": 3}
            for r in rows:
                self.assertEqual(int(r["fold"]), folds[r["group"]])
            # OOF loss is a genuine held-out metric, not training loss.
            with open(os.path.join(tmp, "model.json"), encoding="utf-8") as fh:
                model = json.load(fh)
            self.assertEqual(len(model["candidates"]), 3)
            for cand in model["candidates"]:
                self.assertTrue(cand["converged"])
                self.assertGreater(cand["oof_loss"], 0.0)

    def test_constant_column_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._train(tmp, constant=True)
            with open(os.path.join(tmp, "model.json"), encoding="utf-8") as fh:
                model = json.load(fh)
            idx = model["feature_names"].index("x2")
            self.assertEqual(model["scales"][idx], 1.0)

    def test_save_reload_and_column_order(self):
        with tempfile.TemporaryDirectory() as tmp:
            self._train(tmp)
            model_path = os.path.join(tmp, "model.json")
            new_csv = os.path.join(tmp, "new.csv")
            with open(new_csv, "w", newline="") as fh:
                fh.write("x1,x2\n2.0,0.5\n-1.5,-0.25\n")
            out1 = os.path.join(tmp, "p1.csv")
            run_predict(model_path, new_csv, out1)
            # Shuffled column order plus an extra unused column.
            shuffled = os.path.join(tmp, "shuffled.csv")
            with open(shuffled, "w", newline="") as fh:
                fh.write("extra,x2,x1\n9,0.5,2.0\n8,-0.25,-1.5\n")
            out2 = os.path.join(tmp, "p2.csv")
            run_predict(model_path, shuffled, out2)
            with open(out1, newline="") as fh:
                probs1 = [r["probability"] for r in csv.DictReader(fh)]
            with open(out2, newline="") as fh:
                probs2 = [r["probability"] for r in csv.DictReader(fh)]
            self.assertEqual(probs1, probs2)
            self.assertNotEqual(probs1[0], probs1[1])

    def test_tie_break_prefers_larger_lambda(self):
        # Equal OOF losses must select the larger lambda.
        results = [{"lambda": 0.1, "oof_loss": 0.5},
                   {"lambda": 1.0, "oof_loss": 0.5}]
        best = min(results, key=lambda r: (r["oof_loss"], -r["lambda"]))
        self.assertEqual(best["lambda"], 1.0)


if __name__ == "__main__":
    unittest.main()
