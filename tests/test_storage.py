import hashlib
import json
import os
import shutil
import tempfile
import unittest
from pathlib import Path

from erasure_stripe.storage import (
    ManifestError,
    RestoreError,
    check_manifest,
    encode_file,
    restore_file,
    shard_filename,
)


class StorageCase(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        self.data = bytes((i * 131 + 17) % 256 for i in range(150_000))
        self.src = self.tmp / "source.bin"
        self.src.write_bytes(self.data)
        self.out = self.tmp / "backup"
        self.manifest = encode_file(self.src, self.out, k=4, parity=2, stripe_size=10_000)

    def shard_path(self, stripe, shard):
        return self.out / shard_filename(stripe, shard)

    def restore(self, name="restored.bin"):
        target = self.tmp / name
        restore_file(self.manifest, target)
        return target


class TestRoundTrip(StorageCase):
    def test_clean_restore(self):
        self.assertEqual(self.restore().read_bytes(), self.data)

    def test_empty_file(self):
        empty = self.tmp / "empty.bin"
        empty.write_bytes(b"")
        out = self.tmp / "empty-backup"
        manifest = encode_file(empty, out, k=3, parity=2, stripe_size=512)
        target = self.tmp / "empty-restored.bin"
        restore_file(manifest, target)
        self.assertEqual(target.read_bytes(), b"")

    def test_tail_stripe_no_padding_leak(self):
        odd = self.tmp / "odd.bin"
        payload = os.urandom(10_001)
        odd.write_bytes(payload)
        out = self.tmp / "odd-backup"
        manifest = encode_file(odd, out, k=4, parity=2, stripe_size=4_096)
        target = self.tmp / "odd-restored.bin"
        restore_file(manifest, target)
        self.assertEqual(target.read_bytes(), payload)

    def test_missing_shards_any_position(self):
        for stripe in range(15):
            self.shard_path(stripe, 1).unlink()
            self.shard_path(stripe, 4).unlink()
        self.assertEqual(self.restore("r1.bin").read_bytes(), self.data)

    def test_corrupted_shard_detected_and_recovered(self):
        path = self.shard_path(3, 2)
        raw = bytearray(path.read_bytes())
        raw[100] ^= 0xFF
        path.write_bytes(bytes(raw))
        reports = check_manifest(self.manifest)
        bad = [s for s in reports[3].shards if not s.ok]
        self.assertEqual(len(bad), 1)
        self.assertEqual(bad[0].index, 2)
        self.assertEqual(bad[0].reason, "digest mismatch")
        self.assertEqual(self.restore("r2.bin").read_bytes(), self.data)

    def test_insufficient_shards_reports_and_publishes_nothing(self):
        for shard in (0, 1, 2):
            self.shard_path(5, shard).unlink()
        target = self.tmp / "should-not-exist.bin"
        with self.assertRaises(RestoreError) as ctx:
            restore_file(self.manifest, target)
        self.assertIn("stripe 5", str(ctx.exception))
        self.assertIn("[0, 1, 2]", str(ctx.exception))
        self.assertFalse(target.exists())
        self.assertFalse((self.tmp / "should-not-exist.bin.partial").exists())

    def test_restore_does_not_modify_shards(self):
        before = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                  for p in self.out.glob("stripe-*")}
        self.restore("r3.bin")
        after = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in self.out.glob("stripe-*")}
        self.assertEqual(before, after)


class TestManifestConflicts(StorageCase):
    def edit_manifest(self, mutate):
        doc = json.loads(self.manifest.read_text(encoding="utf-8"))
        mutate(doc)
        self.manifest.write_text(json.dumps(doc), encoding="utf-8")

    def test_duplicate_shard_index_rejected(self):
        def mutate(doc):
            doc["stripes"][0]["shards"][1]["index"] = 0
        self.edit_manifest(mutate)
        with self.assertRaises(ManifestError):
            check_manifest(self.manifest)

    def test_field_mismatch_rejected(self):
        self.edit_manifest(lambda doc: doc["field"].update(polynomial="0x187"))
        with self.assertRaises(ManifestError):
            check_manifest(self.manifest)

    def test_matrix_mismatch_rejected(self):
        def mutate(doc):
            doc["generator_matrix"][4][0] ^= 1
        self.edit_manifest(mutate)
        with self.assertRaises(ManifestError):
            check_manifest(self.manifest)

    def test_length_metadata_conflict_rejected(self):
        def mutate(doc):
            doc["stripes"][0]["shards"][0]["length"] += 1
        self.edit_manifest(mutate)
        with self.assertRaises(ManifestError):
            check_manifest(self.manifest)


if __name__ == "__main__":
    unittest.main()
