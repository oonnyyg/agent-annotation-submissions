from __future__ import annotations

import csv
import json
from pathlib import Path

from .core import INFINITY_MARKER, PersistenceResult


def write_reports(result: PersistenceResult, output_dir: str | Path) -> None:
    path = Path(output_dir)
    path.mkdir(parents=True, exist_ok=True)

    with (path / "report.json").open("w", encoding="utf-8") as handle:
        json.dump(result.to_dict(), handle, ensure_ascii=False, indent=2)
        handle.write("\n")

    with (path / "intervals.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(["identifier", "birth", "death", "death_marker"])
        for interval in result.intervals:
            writer.writerow(
                [
                    interval.identifier,
                    interval.birth,
                    "" if interval.death is None else interval.death,
                    "" if interval.death is not None else INFINITY_MARKER,
                ]
            )

    with (path / "merges.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(
            [
                "distance",
                "surviving_component",
                "merged_component",
                "component_count_after",
            ]
        )
        for merge in result.merges:
            if not merge.merged_components:
                writer.writerow(
                    [
                        merge.distance,
                        merge.surviving_component,
                        "",
                        merge.component_count_after,
                    ]
                )
            for merged in merge.merged_components:
                writer.writerow(
                    [
                        merge.distance,
                        merge.surviving_component,
                        merged,
                        merge.component_count_after,
                    ]
                )

    with (path / "scale_groups.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(["scale", "component_count", "representative", "member"])
        for group in result.scale_groups:
            for representative, members in group.groups:
                for member in members:
                    writer.writerow(
                        [
                            group.scale,
                            group.component_count,
                            representative,
                            member,
                        ]
                    )

    with (path / "scale_summary.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(["scale", "component_count", "alive_interval_count"])
        for group in result.scale_groups:
            alive_count = sum(
                interval.is_alive_at(group.scale) for interval in result.intervals
            )
            writer.writerow([group.scale, group.component_count, alive_count])
