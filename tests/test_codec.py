import itertools
import unittest

from erasure_stripe import gf256
from erasure_stripe.codec import InsufficientShardsError, StripeCoder
from erasure_stripe.matrix import generator_matrix, invert, mat_mul, vandermonde


class TestGF256(unittest.TestCase):
    def test_mul_div_inverse(self):
        for a in (1, 2, 53, 200, 255):
            for b in (1, 3, 87, 254):
                self.assertEqual(gf256.div(gf256.mul(a, b), b), a)
        for a in range(1, 256):
            self.assertEqual(gf256.mul(a, gf256.inv(a)), 1)

    def test_generator_order(self):
        self.assertEqual(gf256.pow_(gf256.GENERATOR, 255), 1)
        self.assertNotEqual(gf256.pow_(gf256.GENERATOR, 51), 1)


class TestMatrix(unittest.TestCase):
    def test_invert_identity(self):
        m = vandermonde(4, 4)
        identity = [[1 if i == j else 0 for j in range(4)] for i in range(4)]
        self.assertEqual(mat_mul(m, invert(m)), identity)

    def test_generator_is_systematic(self):
        g = generator_matrix(5, 3)
        self.assertEqual(len(g), 8)
        for i in range(5):
            self.assertEqual(g[i], [1 if j == i else 0 for j in range(5)])


class TestStripeCoder(unittest.TestCase):
    def test_data_shards_are_systematic(self):
        coder = StripeCoder(4, 2, 4096)
        stripe = bytes(range(256)) * 16
        shards = coder.encode_stripe(stripe)
        self.assertEqual(len(shards), 6)
        for i in range(4):
            self.assertEqual(shards[i], stripe[i * 1024:(i + 1) * 1024])

    def test_any_k_of_n_combination_recovers(self):
        k, p = 4, 3
        coder = StripeCoder(k, p, 1000)
        stripe = bytes((i * 37 + 11) % 256 for i in range(1000))
        shards = coder.encode_stripe(stripe)
        for combo in itertools.combinations(range(k + p), k):
            available = {i: shards[i] for i in combo}
            self.assertEqual(coder.decode_stripe(available, len(stripe)), stripe,
                             f"combination {combo} failed")

    def test_middle_shards_missing(self):
        coder = StripeCoder(6, 4, 600)
        stripe = bytes((i * 91 + 7) % 256 for i in range(600))
        shards = coder.encode_stripe(stripe)
        available = {i: s for i, s in enumerate(shards) if i not in (2, 3, 5, 7)}
        self.assertEqual(coder.decode_stripe(available, 600), stripe)

    def test_insufficient_shards(self):
        coder = StripeCoder(4, 2, 400)
        shards = coder.encode_stripe(b"x" * 400)
        with self.assertRaises(InsufficientShardsError):
            coder.decode_stripe({0: shards[0], 1: shards[1], 5: shards[5]}, 400)

    def test_tail_padding_stripped(self):
        coder = StripeCoder(5, 2, 1000)
        stripe = b"tail" * 100
        shards = coder.encode_stripe(stripe)
        self.assertTrue(all(len(s) == 200 for s in shards))
        available = {i: shards[i] for i in (0, 2, 3, 5, 6)}
        self.assertEqual(coder.decode_stripe(available, len(stripe)), stripe)

    def test_layout_validation(self):
        for bad in ((1, 2), (9, 1), (8, 5), (4, 0)):
            with self.assertRaises(ValueError):
                StripeCoder(*bad, 1024)


if __name__ == "__main__":
    unittest.main()
