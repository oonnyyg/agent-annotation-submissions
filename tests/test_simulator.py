from __future__ import annotations

import math
import unittest

from quantum_sim import CircuitError, Simulator, run_diagnostic, sample_circuit


class SimulatorTests(unittest.TestCase):
    def test_h_creates_superposition_and_normalizes(self) -> None:
        simulator = Simulator(1)
        state = simulator.initial_state()
        simulator.apply_gate(state, "H", 0)
        self.assertTrue(abs(state[0] - 1 / math.sqrt(2)) < 1e-12)
        self.assertTrue(abs(state[1] - 1 / math.sqrt(2)) < 1e-12)

    def test_s_preserves_phase_and_h_interference(self) -> None:
        simulator = Simulator(1)
        state = simulator.initial_state()
        simulator.apply_gate(state, "X", 0)
        simulator.apply_gate(state, "S", 0)
        self.assertEqual(state[1], 1j)
        simulator.apply_gate(state, "H", 0)
        self.assertTrue(abs(state[0] - 1j / math.sqrt(2)) < 1e-12)
        self.assertTrue(abs(state[1] + 1j / math.sqrt(2)) < 1e-12)

    def test_cnot_entangles_superposition(self) -> None:
        simulator = Simulator(2)
        state = simulator.initial_state()
        simulator.apply_gate(state, "H", 0)
        simulator.apply_cnot(state, 0, 1)
        self.assertEqual(state[0b00], 1 / math.sqrt(2))
        self.assertEqual(state[0b11], 1 / math.sqrt(2))
        self.assertEqual(state[0b01], 0j)
        self.assertEqual(state[0b10], 0j)

    def test_qubit_zero_is_lowest_state_index_bit(self) -> None:
        simulator = Simulator(2)
        state = simulator.initial_state()
        simulator.apply_gate(state, "X", 0)
        self.assertEqual(state[0b01], 1 + 0j)

    def test_cnot_rejects_same_control_and_target(self) -> None:
        simulator = Simulator(2)
        with self.assertRaises(CircuitError):
            simulator.apply_cnot(simulator.initial_state(), 1, 1)

    def test_entangled_samples_are_correlated(self) -> None:
        circuit = {
            "num_qubits": 2,
            "operations": [
                {"gate": "H", "qubit": 0},
                {"gate": "CNOT", "control": 0, "target": 1},
            ],
        }
        result = sample_circuit(circuit, 200, seed=7)
        self.assertEqual(result.counts["01"], 0)
        self.assertEqual(result.counts["10"], 0)
        self.assertEqual(result.counts["00"] + result.counts["11"], 200)

    def test_mid_measurement_collapses_and_records_outcome(self) -> None:
        circuit = {
            "num_qubits": 1,
            "num_classical_bits": 1,
            "operations": [
                {"gate": "H", "qubit": 0},
                {"gate": "measure", "qubit": 0, "classical_bit": 0},
            ],
        }
        result = run_diagnostic(circuit, seed=1)
        outcome = result.measurements[0]["outcome"]
        nonzero = [value for value in result.state_vector if abs(complex(value["real"], value["imag"])) > 1e-12]
        self.assertIn(outcome, (0, 1))
        self.assertEqual(len(nonzero), 1)

    def test_classical_feedback_can_correct_bit(self) -> None:
        circuit = {
            "num_qubits": 1,
            "num_classical_bits": 1,
            "operations": [
                {"gate": "H", "qubit": 0},
                {"gate": "measure", "qubit": 0, "classical_bit": 0},
                {"gate": "X", "qubit": 0, "condition": {"classical_bit": 0, "equals": 1}},
            ],
        }
        result = sample_circuit(circuit, 50, seed=3)
        self.assertEqual(result.counts["1"], 0)
        self.assertEqual(result.counts["0"], 50)

    def test_failed_condition_leaves_state_unchanged(self) -> None:
        circuit = {
            "num_qubits": 1,
            "num_classical_bits": 1,
            "operations": [{"gate": "X", "qubit": 0, "condition": {"classical_bit": 0, "equals": 1}}],
        }
        result = run_diagnostic(circuit, seed=1)
        self.assertAlmostEqual(result.state_vector[0]["real"], 1.0)
        self.assertEqual(result.final_bitstring, "0")

    def test_seeded_runs_are_reproducible_and_isolated(self) -> None:
        circuit = {
            "num_qubits": 2,
            "operations": [
                {"gate": "H", "qubit": 0},
                {"gate": "CNOT", "control": 0, "target": 1},
            ],
        }
        first = sample_circuit(circuit, 20, seed=123)
        second = sample_circuit(circuit, 20, seed=123)
        self.assertEqual(first.counts, second.counts)

    def test_invalid_indices_and_gates_are_rejected_before_run(self) -> None:
        with self.assertRaises(CircuitError):
            run_diagnostic({"num_qubits": 1, "operations": [{"gate": "H", "qubit": 1}]})
        with self.assertRaises(CircuitError):
            run_diagnostic({"num_qubits": 1, "operations": [{"gate": "NOPE", "qubit": 0}]})
        with self.assertRaises(CircuitError):
            run_diagnostic(
                {
                    "num_qubits": 1,
                    "num_classical_bits": 1,
                    "operations": [{"gate": "X", "qubit": 0, "condition": {"classical_bit": 1, "equals": 0}}],
                }
            )


if __name__ == "__main__":
    unittest.main()
