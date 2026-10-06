"""Trajectory and config loading with validation."""

import json


class ConfigError(ValueError):
    """Raised when the configuration or trajectory is invalid."""


class Frame:
    __slots__ = ("box", "ids", "types", "positions")

    def __init__(self, box, ids, types, positions):
        self.box = box
        self.ids = ids
        self.types = types
        self.positions = positions

    @property
    def volume(self):
        return self.box[0] * self.box[1] * self.box[2]


def load_config(path):
    with open(path, "r", encoding="utf-8") as fh:
        cfg = json.load(fh)
    for key in ("trajectory", "pairs", "r_max", "n_bins"):
        if key not in cfg:
            raise ConfigError(f"config missing required key: {key!r}")
    if not isinstance(cfg["n_bins"], int) or cfg["n_bins"] < 1:
        raise ConfigError("n_bins must be a positive integer")
    if not isinstance(cfg["r_max"], (int, float)) or cfg["r_max"] <= 0:
        raise ConfigError("r_max must be a positive number")
    pairs = []
    for entry in cfg["pairs"]:
        if isinstance(entry, dict):
            a, b = entry.get("a"), entry.get("b")
        else:
            a, b = entry
        if not a or not b:
            raise ConfigError(f"invalid pair entry: {entry!r}")
        pairs.append((str(a), str(b)))
    return {
        "trajectory": cfg["trajectory"],
        "pairs": pairs,
        "r_max": float(cfg["r_max"]),
        "n_bins": cfg["n_bins"],
        "output_csv": cfg.get("output_csv", "pair_distribution.csv"),
        "summary_json": cfg.get("summary_json", "summary.json"),
    }


def load_trajectory(path):
    frames = []
    ref_signature = None
    with open(path, "r", encoding="utf-8") as fh:
        for lineno, line in enumerate(fh, 1):
            line = line.strip()
            if not line:
                continue
            try:
                data = json.loads(line)
            except json.JSONDecodeError as exc:
                raise ConfigError(f"line {lineno}: invalid JSON: {exc}") from exc
            frame = _parse_frame(data, lineno)
            signature = sorted(zip(frame.ids, frame.types))
            if ref_signature is None:
                ref_signature = signature
            elif signature != ref_signature:
                raise ConfigError(
                    f"line {lineno}: particle ids/types differ from first frame"
                )
            frames.append(frame)
    if not frames:
        raise ConfigError("trajectory contains no frames")
    return frames


def _parse_frame(data, lineno):
    box = data.get("box")
    if (
        not isinstance(box, list)
        or len(box) != 3
        or any(not isinstance(v, (int, float)) or v <= 0 for v in box)
    ):
        raise ConfigError(f"line {lineno}: box must be three positive numbers")
    particles = data.get("particles")
    if not isinstance(particles, list) or not particles:
        raise ConfigError(f"line {lineno}: particles must be a non-empty list")
    ids, types, positions = [], [], []
    seen = set()
    for p in particles:
        pid = p.get("id")
        ptype = p.get("type")
        pos = p.get("position")
        if pid is None or ptype is None:
            raise ConfigError(f"line {lineno}: particle missing id or type")
        if pid in seen:
            raise ConfigError(f"line {lineno}: duplicate particle id {pid!r}")
        seen.add(pid)
        if (
            not isinstance(pos, list)
            or len(pos) != 3
            or any(not isinstance(v, (int, float)) for v in pos)
        ):
            raise ConfigError(f"line {lineno}: bad position for id {pid!r}")
        ids.append(pid)
        types.append(str(ptype))
        positions.append(tuple(float(v) for v in pos))
    return Frame([float(v) for v in box], ids, types, positions)
