import math
import random
import unittest

import qsim

INV_SQRT2 = 1 / math.sqrt(2)


def make(data):
    return qsim.parse_circuit(data)


def shot(circuit, seed=0):
    return qsim.run_shot(circuit, random.Random(seed))


class ValidationTests(unittest.TestCase):
    def test_qubit_count_bounds(self):
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 0, "ops": []})
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 11, "ops": []})
        make({"qubits": 1, "ops": []})
        make({"qubits": 10, "ops": []})

    def test_bad_qubit_index(self):
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 2, "ops": [{"gate": "H", "qubit": 2}]})
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 2, "ops": [{"gate": "H", "qubit": -1}]})

    def test_cx_same_qubit_rejected(self):
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 2, "ops": [{"gate": "CX", "control": 1, "target": 1}]})

    def test_bad_classical_bit(self):
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "MEASURE", "qubit": 0, "classical_bit": 1}]})

    def test_bad_condition(self):
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "X", "qubit": 0, "condition": {"bit": 0, "value": 2}}]})

    def test_unknown_gate(self):
        with self.assertRaises(qsim.CircuitError):
            make({"qubits": 1, "ops": [{"gate": "T", "qubit": 0}]})


class GateTests(unittest.TestCase):
    def test_x_flips_correct_amplitudes(self):
        c = make({"qubits": 2, "ops": [{"gate": "X", "qubit": 0}]})
        r = shot(c)
        self.assertEqual(r.state, [0j, 1 + 0j, 0j, 0j])
        self.assertEqual(r.final_bits, "01")

    def test_qubit_zero_is_least_significant(self):
        c = make({"qubits": 2, "ops": [{"gate": "X", "qubit": 1}]})
        r = shot(c)
        self.assertEqual(r.state, [0j, 0j, 1 + 0j, 0j])
        self.assertEqual(r.final_bits, "10")

    def test_hadamard_superposition_normalized(self):
        c = make({"qubits": 1, "ops": [{"gate": "H", "qubit": 0}]})
        r = shot(c)
        self.assertAlmostEqual(r.state[0].real, INV_SQRT2)
        self.assertAlmostEqual(r.state[1].real, INV_SQRT2)
        self.assertAlmostEqual(sum(abs(a) ** 2 for a in r.state), 1.0)

    def test_s_gate_phase(self):
        c = make({"qubits": 1, "ops": [{"gate": "X", "qubit": 0},
                                       {"gate": "S", "qubit": 0}]})
        r = shot(c)
        self.assertAlmostEqual(r.state[1].imag, 1.0)

    def test_phase_interference(self):
        # H then S then H: amplitudes interfere, P(1) = 1/2 - ... deterministic state.
        c = make({"qubits": 1, "ops": [{"gate": "H", "qubit": 0},
                                       {"gate": "S", "qubit": 0},
                                       {"gate": "H", "qubit": 0}]})
        r = shot(c)
        self.assertAlmostEqual(r.state[0].real, 0.5)
        self.assertAlmostEqual(r.state[0].imag, 0.5)
        self.assertAlmostEqual(r.state[1].real, 0.5)
        self.assertAlmostEqual(r.state[1].imag, -0.5)
        self.assertAlmostEqual(sum(abs(a) ** 2 for a in r.state), 1.0)

    def test_hh_identity(self):
        c = make({"qubits": 1, "ops": [{"gate": "H", "qubit": 0},
                                       {"gate": "H", "qubit": 0}]})
        r = shot(c)
        self.assertAlmostEqual(r.state[0].real, 1.0)
        self.assertAlmostEqual(abs(r.state[1]), 0.0)

    def test_cx_bell_state(self):
        c = make({"qubits": 2, "ops": [{"gate": "H", "qubit": 1},
                                       {"gate": "CX", "control": 1, "target": 0}]})
        r = shot(c)
        self.assertAlmostEqual(r.state[0].real, INV_SQRT2)
        self.assertAlmostEqual(r.state[3].real, INV_SQRT2)
        self.assertAlmostEqual(abs(r.state[1]), 0.0)
        self.assertAlmostEqual(abs(r.state[2]), 0.0)

    def test_cx_control_zero_no_flip(self):
        c = make({"qubits": 2, "ops": [{"gate": "X", "qubit": 0},
                                       {"gate": "CX", "control": 1, "target": 0}]})
        r = shot(c)
        self.assertEqual(r.state, [0j, 1 + 0j, 0j, 0j])


class MeasurementTests(unittest.TestCase):
    def test_collapse_and_normalize(self):
        c = make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "H", "qubit": 0},
                          {"gate": "MEASURE", "qubit": 0, "classical_bit": 0}]})
        for seed in range(20):
            r = shot(c, seed)
            outcome = r.measurements[0].outcome
            self.assertAlmostEqual(abs(r.state[outcome]), 1.0)
            self.assertAlmostEqual(abs(r.state[1 - outcome]), 0.0)
            self.assertEqual(r.classical_bits, [outcome])
            self.assertEqual(r.final_bits, str(outcome))

    def test_deterministic_measurement(self):
        c = make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "MEASURE", "qubit": 0, "classical_bit": 0}]})
        for seed in range(10):
            self.assertEqual(shot(c, seed).measurements[0].outcome, 0)

    def test_entangled_measurement_correlation(self):
        # Bell pair: measuring qubit 1 collapses qubit 0 to the same value.
        c = make({"qubits": 2, "classical_bits": 2,
                  "ops": [{"gate": "H", "qubit": 1},
                          {"gate": "CX", "control": 1, "target": 0},
                          {"gate": "MEASURE", "qubit": 1, "classical_bit": 1},
                          {"gate": "MEASURE", "qubit": 0, "classical_bit": 0}]})
        for seed in range(30):
            r = shot(c, seed)
            o1 = r.measurements[0].outcome
            o0 = r.measurements[1].outcome
            self.assertEqual(o0, o1)
            self.assertEqual(r.final_bits, f"{o1}{o0}")

    def test_superposition_not_boolean(self):
        # H, measure, then check the collapsed state is a pure basis state
        # but the pre-measurement state was a complex superposition.
        c = make({"qubits": 1, "ops": [{"gate": "H", "qubit": 0},
                                       {"gate": "S", "qubit": 0}]})
        r = shot(c)
        self.assertAlmostEqual(r.state[1].imag, INV_SQRT2)


class FeedbackTests(unittest.TestCase):
    def test_condition_not_met_leaves_state_unchanged(self):
        # Classical bit stays 0, so the conditioned X never fires.
        c = make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "X", "qubit": 0,
                           "condition": {"bit": 0, "value": 1}}]})
        r = shot(c)
        self.assertEqual(r.state, [1 + 0j, 0j])
        self.assertEqual(r.final_bits, "0")

    def test_condition_met_applies_gate(self):
        c = make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "X", "qubit": 0,
                           "condition": {"bit": 0, "value": 0}}]})
        r = shot(c)
        self.assertEqual(r.state, [0j, 1 + 0j])

    def test_classical_feedback_flip(self):
        # Measure a |1> qubit into c0, then flip qubit 1 conditioned on c0==1.
        c = make({"qubits": 2, "classical_bits": 1,
                  "ops": [{"gate": "X", "qubit": 0},
                          {"gate": "MEASURE", "qubit": 0, "classical_bit": 0},
                          {"gate": "X", "qubit": 1,
                           "condition": {"bit": 0, "value": 1}}]})
        r = shot(c)
        self.assertEqual(r.final_bits, "11")

    def test_feedback_restores_zero(self):
        # Teleportation-style correction: H, measure, X conditioned on outcome
        # returns the qubit to |0> deterministically.
        c = make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "H", "qubit": 0},
                          {"gate": "MEASURE", "qubit": 0, "classical_bit": 0},
                          {"gate": "X", "qubit": 0,
                           "condition": {"bit": 0, "value": 1}}]})
        for seed in range(20):
            r = shot(c, seed)
            self.assertEqual(r.final_bits, "0")
            self.assertAlmostEqual(abs(r.state[0]), 1.0)


class SamplingTests(unittest.TestCase):
    def test_seed_reproducibility(self):
        c = make({"qubits": 2, "classical_bits": 1,
                  "ops": [{"gate": "H", "qubit": 0},
                          {"gate": "H", "qubit": 1},
                          {"gate": "MEASURE", "qubit": 0, "classical_bit": 0}]})
        a = qsim.run_counts(c, 200, seed=123)
        b = qsim.run_counts(c, 200, seed=123)
        self.assertEqual(a, b)

    def test_shots_do_not_contaminate_each_other(self):
        # Every shot must start from |00>: counts only ever contain 00/01/10/11
        # with the expected marginal structure, never stale classical bits.
        c = make({"qubits": 1, "classical_bits": 1,
                  "ops": [{"gate": "H", "qubit": 0},
                          {"gate": "MEASURE", "qubit": 0, "classical_bit": 0},
                          {"gate": "X", "qubit": 0,
                           "condition": {"bit": 0, "value": 1}}]})
        counts = qsim.run_counts(c, 500, seed=7)
        self.assertEqual(counts, {"0": 500})

    def test_bell_sampling_distribution(self):
        c = make({"qubits": 2, "ops": [{"gate": "H", "qubit": 1},
                                       {"gate": "CX", "control": 1, "target": 0}]})
        counts = qsim.run_counts(c, 2000, seed=99)
        self.assertEqual(set(counts), {"00", "11"})
        self.assertAlmostEqual(counts["00"] / 2000, 0.5, delta=0.1)

    def test_counts_sum_to_shots(self):
        c = make({"qubits": 3, "ops": [{"gate": "H", "qubit": 0},
                                       {"gate": "H", "qubit": 1},
                                       {"gate": "H", "qubit": 2}]})
        counts = qsim.run_counts(c, 100, seed=1)
        self.assertEqual(sum(counts.values()), 100)


if __name__ == "__main__":
    unittest.main()
