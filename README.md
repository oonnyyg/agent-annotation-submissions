# Stoichiometric Feed Balance

`python -m reaction_balance` is a standard-library-only tool that first
verifies each chemical equation balances **uniquely**, then tracks the
material inventory through a sequence of feed and reaction steps. It
does **not** compute reaction rates or thermodynamic equilibrium.

## Usage

```
python -m reaction_balance input.json --balance-json balance.json --steps-csv steps.csv
```

- `input.json`: reaction definitions plus the operation sequence (`-`
  reads from stdin).
- `--balance-json`: balancing coefficients and a per-step JSON report
  (default `balance.json`).
- `--steps-csv`: stepwise consumption/production/remaining CSV
  (default `steps.csv`).

All definitions and operations are validated before anything runs; on
any error the tool exits non-zero and writes **no** output files, so a
failed run never leaves fake-complete results behind.

## Input format

```json
{
  "reactions": [
    {"name": "synthesize_water", "reactants": ["H2", "O2"], "products": ["H2O"]}
  ],
  "operations": [
    {"type": "add", "species": "H2", "amount": "5"},
    {"type": "add", "species": "O2", "amount": "2"},
    {"type": "react", "reaction": "synthesize_water", "fraction": "0.5"}
  ]
}
```

- `add`: adds a non-negative molar amount of a species to the inventory.
- `react`: runs a defined reaction. The maximum extent is set by the
  current limiting reactant(s); the actual extent is that maximum times
  `fraction` (0 to 1). Products enter the inventory and can feed later
  steps. Tied limiting reactants are all reported.

## Formula syntax

Element symbols, positive integer subscripts and nested parentheses
(e.g. `Ca(OH)2`, `K4(ON(SO3)2)2`). Ions (`Na+`), hydrate dots
(`CuSO4.5H2O`) and isotopes are rejected, as are unknown elements and
unbalanced parentheses. The formula string is the species identity; a
single equation may not repeat a species.

## Stoichiometric assumptions

- Balancing uses exact rational arithmetic (element-conservation null
  space). Only equations whose null space has dimension exactly one and
  admits an all-positive coefficient vector are accepted; coefficients
  are reduced to coprime positive integers. "No solution" and "not
  unique" are distinct errors. The tool never adds/removes species or
  guesses coefficients.
- Reactions run to the specified fraction of the limiting-reactant
  maximum extent; no kinetics, no equilibrium, no side reactions.
- Inventory may never go negative (it cannot, by construction of the
  maximum extent).
- Decimal feeds (e.g. `"0.1"`) are parsed as exact decimal rationals
  (`Fraction`), so all accounting is exact. After every step the tool
  checks that the element totals in the inventory equal the cumulative
  element totals fed in.

## Output precision

Internal arithmetic is exact (arbitrary-precision rationals). Reports
print integers exactly and non-integers as decimals rounded to 6
fractional digits (`OUTPUT_DECIMALS` in `reaction_balance/__main__.py`).
The JSON report contains balancing coefficients and per-step
consumed/produced/limiting/remaining; the CSV lists the same per-step
data with one `remaining[<species>]` column per species.

## Tests

```
python -m unittest discover -s tests
```

Covers parenthesised-formula balancing, non-unique equations
(`C + O2 -> CO + CO2`), product reuse in later reactions, exact decimal
feeds, and the no-output-on-error guarantee.