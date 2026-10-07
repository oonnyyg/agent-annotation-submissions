"""Streaming file encode/check/recover plus JSON manifest handling."""

import hashlib
import json
import os

from .codec import decode_stripe, encode_stripe
from .gf256 import FIELD_POLY, FIELD_SIZE, GENERATOR
from .matrix import build_generator

FORMAT = "erasure-stripe/1"
DEFAULT_STRIPE_SIZE = 65536
MANIFEST_NAME = "manifest.json"


class CheckError(Exception):
    """Raised for metadata conflicts or unreadable shard sets."""


def shard_filename(stripe_index, shard_index):
    return f"stripe-{stripe_index:06d}-{shard_index:02d}.bin"


def _sha256(data):
    return hashlib.sha256(data).hexdigest()


def encode_file(src_path, out_dir, k, m, stripe_size=DEFAULT_STRIPE_SIZE):
    """Encode src_path into shard files plus a JSON manifest in out_dir.

    Reads the input one stripe (k * stripe_size bytes) at a time so the
    whole file is never held in memory. The final partial stripe is
    zero-padded; the manifest records the true length so padding never
    leaks into recovered output.
    """
    if stripe_size < 1:
        raise ValueError("stripe_size must be positive")
    generator = build_generator(k, m)
    os.makedirs(out_dir, exist_ok=True)
    file_hash = hashlib.sha256()
    total = 0
    stripes = []
    with open(src_path, "rb") as fh:
        stripe_index = 0
        while True:
            chunks = [fh.read(stripe_size) for _ in range(k)]
            if not any(chunks):
                break
            data_shards = [
                c + b"\x00" * (stripe_size - len(c)) for c in chunks
            ]
            consumed = sum(len(c) for c in chunks)
            file_hash.update(b"".join(chunks))
            total += consumed
            shards = encode_stripe(data_shards, generator)
            shard_entries = []
            for shard_index, shard in enumerate(shards):
                name = shard_filename(stripe_index, shard_index)
                with open(os.path.join(out_dir, name), "wb") as out:
                    out.write(shard)
                shard_entries.append(
                    {
                        "index": shard_index,
                        "file": name,
                        "length": len(shard),
                        "sha256": _sha256(shard),
                    }
                )
            stripes.append({"index": stripe_index, "shards": shard_entries})
            stripe_index += 1
    manifest = {
        "format": FORMAT,
        "field": {
            "size": FIELD_SIZE,
            "polynomial": hex(FIELD_POLY),
            "generator": hex(GENERATOR),
        },
        "data_shards": k,
        "parity_shards": m,
        "stripe_size": stripe_size,
        "generator_matrix": generator,
        "original_length": total,
        "original_sha256": file_hash.hexdigest(),
        "stripes": stripes,
    }
    with open(os.path.join(out_dir, MANIFEST_NAME), "w", encoding="utf-8") as fh:
        json.dump(manifest, fh, indent=2)
    return manifest


def load_manifest(directory):
    """Load and structurally validate the manifest; reject conflicts."""
    path = os.path.join(directory, MANIFEST_NAME)
    with open(path, "r", encoding="utf-8") as fh:
        manifest = json.load(fh)
    if manifest.get("format") != FORMAT:
        raise CheckError(f"unsupported manifest format: {manifest.get('format')!r}")
    field = manifest.get("field", {})
    expected = {
        "size": FIELD_SIZE,
        "polynomial": hex(FIELD_POLY),
        "generator": hex(GENERATOR),
    }
    if field != expected:
        raise CheckError(f"field convention mismatch: {field!r} != {expected!r}")
    k = manifest["data_shards"]
    m = manifest["parity_shards"]
    if manifest.get("generator_matrix") != build_generator(k, m):
        raise CheckError("generator matrix in manifest does not match k/m")
    seen_stripes = set()
    for stripe in manifest["stripes"]:
        if stripe["index"] in seen_stripes:
            raise CheckError(f"duplicate stripe index {stripe['index']}")
        seen_stripes.add(stripe["index"])
        seen_shards = set()
        for shard in stripe["shards"]:
            if shard["index"] in seen_shards:
                raise CheckError(
                    f"duplicate shard index {shard['index']} "
                    f"in stripe {stripe['index']}"
                )
            seen_shards.add(shard["index"])
            if not 0 <= shard["index"] < k + m:
                raise CheckError(f"shard index {shard['index']} out of range")
    return manifest


def verify_stripe_shards(directory, stripe, stripe_size):
    """Return (valid, invalid) shard-index -> bytes maps for one stripe.

    A shard is valid only if its file exists, its length matches the
    manifest, and its SHA-256 matches. Missing or corrupt shards are
    reported in the invalid map with a reason string.
    """
    valid = {}
    invalid = {}
    for entry in stripe["shards"]:
        idx = entry["index"]
        if idx in valid or idx in invalid:
            continue
        path = os.path.join(directory, entry["file"])
        try:
            with open(path, "rb") as fh:
                data = fh.read()
        except OSError:
            invalid[idx] = "missing file"
            continue
        if len(data) != entry["length"] or len(data) != stripe_size:
            invalid[idx] = (
                f"length {len(data)} != expected {entry['length']}"
            )
            continue
        if _sha256(data) != entry["sha256"]:
            invalid[idx] = "sha256 mismatch"
            continue
        valid[idx] = data
    return valid, invalid


def check_directory(directory):
    """Verify every shard; return a report dict."""
    manifest = load_manifest(directory)
    k = manifest["data_shards"]
    stripe_size = manifest["stripe_size"]
    report = {"ok": True, "stripes": []}
    for stripe in manifest["stripes"]:
        valid, invalid = verify_stripe_shards(directory, stripe, stripe_size)
        recoverable = len(valid) >= k
        if invalid or not recoverable:
            report["ok"] = False
        report["stripes"].append(
            {
                "index": stripe["index"],
                "valid": sorted(valid),
                "invalid": {i: reason for i, reason in sorted(invalid.items())},
                "recoverable": recoverable,
            }
        )
    return manifest, report


def recover_file(directory, target_path):
    """Rebuild the original file from valid shards.

    The recovered bytes are streamed to a temporary file one stripe at a
    time; the whole-file SHA-256 is verified against the manifest before
    the target is published. On any failure nothing is written to the
    target and existing shards are left untouched.
    """
    manifest = load_manifest(directory)
    k = manifest["data_shards"]
    m = manifest["parity_shards"]
    stripe_size = manifest["stripe_size"]
    remaining = manifest["original_length"]
    file_hash = hashlib.sha256()
    tmp_path = target_path + ".tmp-recover"
    try:
        with open(tmp_path, "wb") as out:
            for stripe in manifest["stripes"]:
                valid, invalid = verify_stripe_shards(
                    directory, stripe, stripe_size
                )
                if len(valid) < k:
                    missing = sorted(set(range(k + m)) - set(valid))
                    raise CheckError(
                        f"stripe {stripe['index']}: only {len(valid)} of "
                        f"{k} required shards valid; unusable shard "
                        f"indexes: {missing} ({invalid})"
                    )
                data_shards = decode_stripe(valid, k, m)
                for shard in data_shards:
                    take = min(remaining, len(shard))
                    if take:
                        out.write(shard[:take])
                        file_hash.update(shard[:take])
                        remaining -= take
        if remaining != 0:
            raise CheckError("manifest stripes do not cover original length")
        if file_hash.hexdigest() != manifest["original_sha256"]:
            raise CheckError("recovered content fails whole-file sha256 check")
        os.replace(tmp_path, target_path)
    except BaseException:
        if os.path.exists(tmp_path):
            os.remove(tmp_path)
        raise
    return manifest
