# group_logit

Binary logistic regression trainer with **group-aware cross-validation** for
regularization-strength selection. Pure Python standard library; no
third-party dependencies.

## Usage

Train (writes out-of-fold validation predictions and a reusable model):

```
python -m group_logit train --data train.csv --config config.json \
    --predictions val_predictions.csv --model model.json [--report report.json]
```

Predict with a saved model:

```
python -m group_logit predict --model model.json --data new.csv --output predictions.csv
```

## Input data

CSV with a header row. One column is the label (values must be exactly `0` or
`1`), one column is the group id, and every other column is a numeric feature.
Rows with unknown groups, missing values, or non-finite features (NaN/inf)
are rejected with an error.

## Config (JSON)

```json
{
  "label_column": "y",
  "group_column": "grp",
  "fold_assignments": {"g1": 0, "g2": 0, "g3": 1, "g4": 1},
  "lambdas": [0.01, 0.1, 1.0],
  "max_iterations": 200,
  "tolerance": 1e-8
}
```

- `fold_assignments` maps each group to exactly one validation fold, so all
  rows of a group appear together in either the training part or the
  validation part of every split — groups never straddle both sides. Every
  training split must contain both classes.
- `lambdas` are the candidate regularization strengths; all must be positive.
- `max_iterations` / `tolerance` are optional (defaults 200 and 1e-8).

## Model and objective

The model is logistic regression with an intercept. The minimized objective
is

```
mean binary cross-entropy  +  lambda * (1/2) * sum(w_j^2)
```

where the penalty applies only to the feature coefficients; the intercept is
never penalized. `lambda` therefore scales the coefficient penalty relative
to a per-sample (mean) loss, so its reasonable magnitude does not depend on
the dataset size.

### Optimization

Newton's method (IRLS): each iteration forms the gradient and Hessian of the
regularized objective and solves the Newton system by Gaussian elimination
with partial pivoting, followed by a backtracking line search that only
accepts steps that decrease the objective. Convergence is declared when the
max absolute gradient component falls below `tolerance`. If any fit (any
fold, any candidate, or the final refit) does not converge within
`max_iterations`, training fails with an explicit error and **no model file
is written** — a non-converged model is never passed off as valid.

The sigmoid and log-loss are computed in a numerically stable form
(branching on the sign of the score, softplus via `log1p(exp(-|z|))`), so
arbitrarily large scores cannot overflow or silently truncate training.

### Preprocessing

For each fold split, feature means and **population** standard deviations
(dividing by n) are computed on the training part only and applied to the
validation part unchanged — validation data never influences preprocessing.
Constant columns get a scale of 1.0. The final model refits preprocessing on
the full dataset and stores it in `model.json`. At prediction time, columns
are matched **by name**, so reordering columns in new data does not change
predictions.

## Selection and reporting

Each candidate lambda is evaluated by the mean out-of-fold cross-entropy
over **all** samples (each row is predicted exactly once, by the fold model
that did not train on its group). The lambda with the lowest loss wins; ties
are broken in favor of the larger (stronger) lambda. The selected lambda is
then refit on the full dataset and saved.

The optional `--report` JSON lists every candidate's out-of-fold loss and
per-fold iteration/convergence status, plus the final fit's status. The
reported losses are out-of-fold (validation) numbers — training-set
performance is never reported as validation performance.

## Model JSON

`model.json` contains `format_version`, the selected `lambda`, `intercept`,
`coefficients`, and the `preprocessing` block (feature names, means,
scales). It is self-contained and can be loaded by
`python -m group_logit predict` on any compatible machine.

## Tests

```
python -m unittest discover -s tests -v
```

Covers group isolation, constant columns, save/reload round-trip, column
reordering invariance, rejection of invalid inputs, and non-convergence
failure behavior.
