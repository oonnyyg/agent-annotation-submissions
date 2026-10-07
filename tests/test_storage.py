import hashlib
import itertools
import json
import os
import shutil
import tempfile
import unittest

from erasure_stripe import storage
from erasure_stripe.storage import CheckError


class StorageTestBase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        self.src = os.path.join(self.tmp, "src.bin")
        self.out = os.path.join(self.tmp, "shards")

    def _write_src(self, data):
        with open(self.src, "wb") as fh:
            fh.write(data)
        return hashlib.sha256(data).hexdigest()

    def _encode(self, data, k=4, m=2, stripe_size=64):
        digest = self._write_src(data)
        manifest = storage.encode_file(self.src, self.out, k, m, stripe_size)
        return digest, manifest

    def _shard_path(self, stripe, shard):
        return os.path.join(self.out, storage.shard_filename(stripe, shard))

    def _recover_bytes(self):
        target = os.path.join(self.tmp, "recovered.bin")
        storage.recover_file(self.out, target)
        with open(target, "rb") as fh:
            return fh.read()


class RoundtripTests(StorageTestBase):
    def test_roundtrip_all_shards_present(self):
        data = os.urandom(1000)
        self._encode(data)
        self.assertEqual(self._recover_bytes(), data)
        _, report = storage.check_directory(self.out)
        self.assertTrue(report["ok"])

    def test_empty_file(self):
        digest, manifest = self._encode(b"")
        self.assertEqual(manifest["original_length"], 0)
        self.assertEqual(manifest["stripes"], [])
        self.assertEqual(
            digest,
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
        )
        self.assertEqual(self._recover_bytes(), b"")

    def test_tail_padding_not_in_output(self):
        data = os.urandom(100)
        self._encode(data, k=4, m=2, stripe_size=64)
        recovered = self._recover_bytes()
        self.assertEqual(recovered, data)
        self.assertEqual(len(recovered), 100)

    def test_every_erasure_combination_recovers(self):
        k, m = 4, 3
        data = os.urandom(4 * 32)
        self._encode(data, k=k, m=m, stripe_size=32)
        names = [storage.shard_filename(0, i) for i in range(k + m)]
        for drop in itertools.combinations(names, m):
            staging = os.path.join(self.tmp, "staging")
            shutil.copytree(self.out, staging)
            for name in drop:
                os.remove(os.path.join(staging, name))
            target = os.path.join(self.tmp, "rec.bin")
            storage.recover_file(staging, target)
            with open(target, "rb") as fh:
                self.assertEqual(fh.read(), data)
            shutil.rmtree(staging)
            os.remove(target)

    def test_middle_shard_loss_not_just_tail(self):
        k, m = 5, 2
        data = os.urandom(5 * 16)
        self._encode(data, k=k, m=m, stripe_size=16)
        os.remove(self._shard_path(0, 1))
        os.remove(self._shard_path(0, 5))
        self.assertEqual(self._recover_bytes(), data)


class CorruptionTests(StorageTestBase):
    def test_corrupt_shard_detected_and_recovered(self):
        k, m = 4, 2
        data = os.urandom(4 * 32)
        self._encode(data, k=k, m=m, stripe_size=32)
        path = self._shard_path(0, 2)
        with open(path, "r+b") as fh:
            fh.write(b"\xff" * 32)
        _, report = storage.check_directory(self.out)
        self.assertFalse(report["ok"])
        stripe = report["stripes"][0]
        self.assertEqual(stripe["invalid"], {2: "sha256 mismatch"})
        self.assertTrue(stripe["recoverable"])
        self.assertEqual(self._recover_bytes(), data)

    def test_truncated_shard_detected(self):
        data = os.urandom(4 * 32)
        self._encode(data, k=4, m=2, stripe_size=32)
        path = self._shard_path(0, 0)
        with open(path, "r+b") as fh:
            fh.truncate(10)
        _, report = storage.check_directory(self.out)
        self.assertIn("length", report["stripes"][0]["invalid"][0])

    def test_insufficient_shards_no_output(self):
        k, m = 4, 2
        data = os.urandom(4 * 32)
        self._encode(data, k=k, m=m, stripe_size=32)
        for i in range(4):
            os.remove(self._shard_path(0, i))
        target = os.path.join(self.tmp, "should_not_exist.bin")
        with self.assertRaises(CheckError) as ctx:
            storage.recover_file(self.out, target)
        self.assertIn("unusable shard indexes", str(ctx.exception))
        self.assertFalse(os.path.exists(target))
        self.assertFalse(os.path.exists(target + ".tmp-recover"))

    def test_shards_not_modified_by_failed_recover(self):
        data = os.urandom(4 * 32)
        self._encode(data, k=4, m=2, stripe_size=32)

        def snapshot():
            result = {}
            for name in os.listdir(self.out):
                with open(os.path.join(self.out, name), "rb") as fh:
                    result[name] = hashlib.sha256(fh.read()).hexdigest()
            return result

        before = snapshot()
        os.remove(self._shard_path(0, 0))
        os.remove(self._shard_path(0, 1))
        os.remove(self._shard_path(0, 2))
        with self.assertRaises(CheckError):
            storage.recover_file(self.out, os.path.join(self.tmp, "x.bin"))
        after = snapshot()
        self.assertEqual(before.keys() - after.keys(), {
            storage.shard_filename(0, 0),
            storage.shard_filename(0, 1),
            storage.shard_filename(0, 2),
        })
        for name in after:
            self.assertEqual(before[name], after[name])


class ManifestConflictTests(StorageTestBase):
    def _load_manifest(self):
        with open(os.path.join(self.out, storage.MANIFEST_NAME), encoding="utf-8") as fh:
            return json.load(fh)

    def _save_manifest(self, manifest):
        with open(os.path.join(self.out, storage.MANIFEST_NAME), "w", encoding="utf-8") as fh:
            json.dump(manifest, fh)

    def test_duplicate_shard_index_rejected(self):
        self._encode(os.urandom(128), k=2, m=1, stripe_size=64)
        manifest = self._load_manifest()
        dup = dict(manifest["stripes"][0]["shards"][0])
        manifest["stripes"][0]["shards"].append(dup)
        self._save_manifest(manifest)
        with self.assertRaises(CheckError):
            storage.check_directory(self.out)

    def test_field_mismatch_rejected(self):
        self._encode(os.urandom(128), k=2, m=1, stripe_size=64)
        manifest = self._load_manifest()
        manifest["field"]["polynomial"] = "0x11d"
        self._save_manifest(manifest)
        with self.assertRaises(CheckError):
            storage.check_directory(self.out)

    def test_generator_matrix_mismatch_rejected(self):
        self._encode(os.urandom(128), k=2, m=1, stripe_size=64)
        manifest = self._load_manifest()
        manifest["generator_matrix"][2] = [1, 1]
        self._save_manifest(manifest)
        with self.assertRaises(CheckError):
            storage.check_directory(self.out)

    def test_tampered_original_hash_fails_recover(self):
        data = os.urandom(128)
        self._encode(data, k=2, m=1, stripe_size=64)
        manifest = self._load_manifest()
        manifest["original_sha256"] = "0" * 64
        self._save_manifest(manifest)
        target = os.path.join(self.tmp, "out.bin")
        with self.assertRaises(CheckError):
            storage.recover_file(self.out, target)
        self.assertFalse(os.path.exists(target))


if __name__ == "__main__":
    unittest.main()
