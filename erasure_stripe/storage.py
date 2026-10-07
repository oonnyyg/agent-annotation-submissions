"""File-level encode/check/restore with a JSON manifest.

Shard files live next to the manifest and are named
``stripe-<i:06d>.shard-<j:02d>.bin``. Encoding streams the input one stripe
at a time; restore streams decoded stripes into a temporary file and only
publishes the target after the whole-file digest matches.
"""

from __future__ import annotations

import hashlib
import json
import os
from dataclasses import dataclass, field
from pathlib import Path

from .codec import InsufficientShardsError, StripeCoder
from .matrix import generator_matrix

FORMAT = "erasure-stripe/1"
FIELD_INFO = {"size": 256, "polynomial": "0x11B", "generator": 3}


class ManifestError(Exception):
    """Manifest is malformed, inconsistent, or conflicts with the tool."""


class RestoreError(Exception):
    """Restore failed; no output file was published."""


def shard_filename(stripe_index: int, shard_index: int) -> str:
    return f"stripe-{stripe_index:06d}.shard-{shard_index:02d}.bin"


def encode_file(source: Path, out_dir: Path, k: int, parity: int,
                stripe_size: int = 1 << 16) -> Path:
    """Encode `source` into shard files + manifest.json inside `out_dir`."""
    coder = StripeCoder(k, parity, stripe_size)
    out_dir.mkdir(parents=True, exist_ok=True)
    total_len = 0
    file_hash = hashlib.sha256()
    stripes = []
    with open(source, "rb") as fh:
        index = 0
        while True:
            stripe = fh.read(stripe_size)
            if not stripe and index > 0:
                break
            shards = coder.encode_stripe(stripe)
            entries = []
            for shard_idx, data in enumerate(shards):
                name = shard_filename(index, shard_idx)
                (out_dir / name).write_bytes(data)
                entries.append({
                    "index": shard_idx,
                    "file": name,
                    "length": len(data),
                    "sha256": hashlib.sha256(data).hexdigest(),
                })
            stripes.append({"index": index, "data_length": len(stripe), "shards": entries})
            total_len += len(stripe)
            file_hash.update(stripe)
            index += 1
            if not stripe:
                break
    manifest = {
        "format": FORMAT,
        "field": FIELD_INFO,
        "layout": {
            "data_shards": k,
            "parity_shards": parity,
            "total_shards": coder.n,
            "stripe_size": stripe_size,
            "shard_length": coder.shard_len,
        },
        "generator_matrix": coder.generator,
        "source": {
            "name": source.name,
            "length": total_len,
            "sha256": file_hash.hexdigest(),
        },
        "stripes": stripes,
    }
    manifest_path = out_dir / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    return manifest_path


@dataclass
class ShardStatus:
    index: int
    ok: bool
    reason: str = ""


@dataclass
class StripeReport:
    index: int
    data_length: int
    shards: list[ShardStatus] = field(default_factory=list)

    @property
    def usable(self) -> list[int]:
        return [s.index for s in self.shards if s.ok]

    @property
    def unusable(self) -> list[int]:
        return [s.index for s in self.shards if not s.ok]


def load_manifest(manifest_path: Path) -> dict:
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ManifestError(f"cannot read manifest: {exc}") from exc
    if manifest.get("format") != FORMAT:
        raise ManifestError(f"unsupported format: {manifest.get('format')!r}")
    if manifest.get("field") != FIELD_INFO:
        raise ManifestError(f"field convention mismatch: {manifest.get('field')!r}")
    layout = manifest.get("layout")
    if not isinstance(layout, dict):
        raise ManifestError("manifest missing layout")
    try:
        coder = StripeCoder(layout["data_shards"], layout["parity_shards"],
                            layout["stripe_size"])
    except (KeyError, ValueError) as exc:
        raise ManifestError(f"invalid layout: {exc}") from exc
    if layout.get("total_shards") != coder.n or layout.get("shard_length") != coder.shard_len:
        raise ManifestError("layout totals inconsistent with data/parity/stripe size")
    if manifest.get("generator_matrix") != generator_matrix(coder.k, coder.parity):
        raise ManifestError("generator matrix does not match the field/layout convention")
    return manifest


def _verify_stripe_entry(entry: dict, coder: StripeCoder, expected_index: int) -> None:
    if entry.get("index") != expected_index:
        raise ManifestError(
            f"stripe index conflict: expected {expected_index}, got {entry.get('index')}")
    shards = entry.get("shards")
    if not isinstance(shards, list) or len(shards) != coder.n:
        raise ManifestError(f"stripe {expected_index}: expected {coder.n} shard entries")
    seen = set()
    for shard in shards:
        idx = shard.get("index")
        if idx in seen:
            raise ManifestError(f"stripe {expected_index}: duplicate shard index {idx}")
        seen.add(idx)
        if not isinstance(idx, int) or not 0 <= idx < coder.n:
            raise ManifestError(f"stripe {expected_index}: shard index {idx!r} out of range")
        if shard.get("length") != coder.shard_len:
            raise ManifestError(f"stripe {expected_index} shard {idx}: length metadata conflict")
        if shard.get("file") != shard_filename(expected_index, idx):
            raise ManifestError(f"stripe {expected_index} shard {idx}: unexpected filename")


def check_manifest(manifest_path: Path) -> list[StripeReport]:
    """Verify every shard's length and digest; return per-stripe reports."""
    manifest = load_manifest(manifest_path)
    base = manifest_path.parent
    layout = manifest["layout"]
    coder = StripeCoder(layout["data_shards"], layout["parity_shards"], layout["stripe_size"])
    reports = []
    for expected_index, entry in enumerate(manifest["stripes"]):
        _verify_stripe_entry(entry, coder, expected_index)
        report = StripeReport(index=expected_index, data_length=entry["data_length"])
        for shard in entry["shards"]:
            idx = shard["index"]
            path = base / shard["file"]
            status = ShardStatus(index=idx, ok=False)
            try:
                data = path.read_bytes()
            except OSError:
                status.reason = "missing"
            else:
                if len(data) != shard["length"]:
                    status.reason = "length mismatch"
                elif hashlib.sha256(data).hexdigest() != shard["sha256"]:
                    status.reason = "digest mismatch"
                else:
                    status.ok = True
            report.shards.append(status)
        reports.append(report)
    return reports


def restore_file(manifest_path: Path, target: Path) -> None:
    """Rebuild the original file; publish `target` only after digest matches."""
    manifest = load_manifest(manifest_path)
    base = manifest_path.parent
    layout = manifest["layout"]
    k = layout["data_shards"]
    coder = StripeCoder(k, layout["parity_shards"], layout["stripe_size"])
    reports = check_manifest(manifest_path)
    tmp = target.with_name(target.name + ".partial")
    file_hash = hashlib.sha256()
    total = 0
    try:
        with open(tmp, "wb") as out:
            for entry, report in zip(manifest["stripes"], reports):
                available = {}
                for shard in entry["shards"]:
                    if shard["index"] in report.usable:
                        available[shard["index"]] = (base / shard["file"]).read_bytes()
                try:
                    data = coder.decode_stripe(available, entry["data_length"])
                except InsufficientShardsError as exc:
                    raise RestoreError(
                        f"stripe {report.index}: only {len(available)} of {k} required "
                        f"shards usable; unusable shards: {report.unusable}"
                    ) from exc
                out.write(data)
                file_hash.update(data)
                total += len(data)
        source = manifest["source"]
        if total != source["length"] or file_hash.hexdigest() != source["sha256"]:
            raise RestoreError("restored content failed the whole-file digest check")
        os.replace(tmp, target)
    finally:
        if tmp.exists():
            tmp.unlink()
