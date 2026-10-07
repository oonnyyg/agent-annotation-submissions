import itertools
import os
import random
import unittest

from erasure_stripe.codec import decode_stripe, encode_stripe
from erasure_stripe.matrix import build_generator, mat_inverse, mat_mul


class MatrixTests(unittest.TestCase):
    def test_generator_is_systematic(self):
        for k in range(2, 9):
            g = build_generator(k, 12 - k)
            for i in range(k):
                self.assertEqual(g[i], [1 if j == i else 0 for j in range(k)])

    def test_every_k_row_subset_is_invertible(self):
        k, m = 5, 4
        g = build_generator(k, m)
        for subset in itertools.combinations(range(k + m), k):
            inv = mat_inverse([g[i] for i in subset])
            prod = mat_mul([g[i] for i in subset], inv)
            self.assertEqual(
                prod, [[1 if i == j else 0 for j in range(k)] for i in range(k)]
            )

    def test_parameter_limits(self):
        with self.assertRaises(ValueError):
            build_generator(1, 2)
        with self.assertRaises(ValueError):
            build_generator(9, 1)
        with self.assertRaises(ValueError):
            build_generator(8, 5)
        with self.assertRaises(ValueError):
            build_generator(4, 0)


class CodecTests(unittest.TestCase):
    def test_all_erasure_combinations(self):
        k, m, size = 4, 3, 50
        g = build_generator(k, m)
        data = [os.urandom(size) for _ in range(k)]
        enc = encode_stripe(data, g)
        self.assertEqual(enc[:k], [bytes(d) for d in data])
        for keep in itertools.combinations(range(k + m), k):
            av = {i: enc[i] for i in keep}
            self.assertEqual(decode_stripe(av, k, m), [bytes(d) for d in data])

    def test_randomized_roundtrip(self):
        rng = random.Random(42)
        for _ in range(300):
            k = rng.randint(2, 8)
            m = rng.randint(1, 12 - k)
            g = build_generator(k, m)
            size = rng.randint(0, 40)
            data = [os.urandom(size) for _ in range(k)]
            enc = encode_stripe(data, g)
            keep = rng.sample(range(k + m), k)
            av = {i: enc[i] for i in keep}
            self.assertEqual(decode_stripe(av, k, m), [bytes(d) for d in data])

    def test_not_xor_only(self):
        # With k=2, m=1 a plain-XOR scheme gives parity = d0 ^ d1 and the
        # row would be [1, 1]; RS parity rows must be nontrivial.
        g = build_generator(2, 1)
        self.assertNotEqual(g[2], [1, 1])

    def test_insufficient_shards_reports_missing(self):
        k, m = 3, 2
        g = build_generator(k, m)
        enc = encode_stripe([os.urandom(10) for _ in range(k)], g)
        with self.assertRaises(ValueError) as ctx:
            decode_stripe({0: enc[0], 4: enc[4]}, k, m)
        self.assertIn("missing shard indexes", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
