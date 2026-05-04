#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
"""
bench/aggregate.py — consolidate hyperfine per-cell JSONs into one
matrix report.

Per-cell hyperfine output (.json / .md) under bench/results/ is raw
tool output; this script reads it back and produces a single
human-readable report covering every cell of one matrix run.

Usage:

    bench/aggregate.py <metadata.txt> [<metadata.txt> ...]
    bench/aggregate.py --since <marker-file>           # find by mtime
    bench/aggregate.py --glob 'YYYYMMDD-HHMMSS.h*.metadata.txt'

Writes <out>.report.md alongside the inputs. With --since, <out> is
derived from the earliest metadata file's timestamp.
"""

from __future__ import annotations

import argparse
import json
import re
import statistics
import sys
from dataclasses import dataclass
from pathlib import Path

CELL_RE = re.compile(
    r"^(?P<ts>\d{8}-\d{6})\.h(?P<hosts>\d+)\.(?P<net>lan|wan\d+)\.metadata\.txt$"
)


@dataclass
class CellResult:
    ts: str
    hosts: int
    network: str               # "lan" or "wanNNN"
    metadata: dict[str, str]   # parsed metadata.txt
    timings: dict[tuple[str, str], dict]  # (phase, tool) -> stats

    @property
    def network_label(self) -> str:
        if self.network == "lan":
            return "LAN"
        return f"WAN{self.network[3:]}ms"

    @property
    def sort_key(self) -> tuple[int, int]:
        net_order = 0 if self.network == "lan" else int(self.network[3:])
        return (self.hosts, net_order)


def parse_metadata(path: Path) -> dict[str, str]:
    md: dict[str, str] = {}
    for line in path.read_text().splitlines():
        if ":" in line and not line.startswith("#"):
            k, _, v = line.partition(":")
            md[k.strip()] = v.strip()
    return md


def load_cell(metadata_path: Path) -> CellResult:
    m = CELL_RE.match(metadata_path.name)
    if not m:
        raise ValueError(f"unrecognised metadata filename: {metadata_path.name}")
    ts, hosts, network = m.group("ts"), int(m.group("hosts")), m.group("net")

    # Strip ".metadata.txt" off the basename. Path.with_suffix("")
    # chained would also strip ".lan" / ".wan100", which we want to keep.
    base = metadata_path.parent / metadata_path.name.removesuffix(".metadata.txt")
    timings: dict[tuple[str, str], dict] = {}
    for phase in ("cold", "warm"):
        for tool in ("scampi", "ansible"):
            jp = base.with_name(f"{base.name}.{phase}.{tool}.json")
            if not jp.exists():
                continue
            data = json.loads(jp.read_text())
            r = data["results"][0]
            times = sorted(r["times"])
            timings[(phase, tool)] = {
                "mean": r["mean"],
                "stddev": r["stddev"],
                "median": r.get("median", statistics.median(times)),
                "min": r["min"],
                "max": r["max"],
                "p95": times[int(len(times) * 0.95)],
                "n": len(times),
            }

    return CellResult(
        ts=ts,
        hosts=hosts,
        network=network,
        metadata=parse_metadata(metadata_path),
        timings=timings,
    )


def fmt_secs(t: float) -> str:
    """Human-friendly seconds with 2-3 sig figs."""
    if t < 1.0:
        return f"{t * 1000:.0f} ms"
    return f"{t:.2f} s"


def fmt_cell(stats: dict | None) -> str:
    if stats is None:
        return "—"
    return f"{fmt_secs(stats['mean'])} ± {fmt_secs(stats['stddev'])}"


def winner(scampi: dict | None, ansible: dict | None) -> str:
    if scampi is None or ansible is None:
        return "—"
    s, a = scampi["mean"], ansible["mean"]
    if s < a:
        return f"**scampi** {a / s:.2f}×"
    return f"**ansible** {s / a:.2f}×"


def render_phase_table(cells: list[CellResult], phase: str) -> list[str]:
    lines = [
        f"### {phase.capitalize()} deploys",
        "",
        "| hosts | network | scampi          | ansible         | winner          |",
        "|------:|:--------|:----------------|:----------------|:----------------|",
    ]
    for c in cells:
        s = c.timings.get((phase, "scampi"))
        a = c.timings.get((phase, "ansible"))
        lines.append(
            f"| {c.hosts:>5} | {c.network_label:<7} "
            f"| {fmt_cell(s):<15} | {fmt_cell(a):<15} | {winner(s, a):<15} |"
        )
    lines.append("")
    return lines


def render_detail_table(cells: list[CellResult], phase: str) -> list[str]:
    lines = [
        f"### {phase.capitalize()} — full distribution (median / p95 / min / max)",
        "",
        "| hosts | network | tool    | median  | p95     | min     | max     |",
        "|------:|:--------|:--------|:--------|:--------|:--------|:--------|",
    ]
    for c in cells:
        for tool in ("scampi", "ansible"):
            s = c.timings.get((phase, tool))
            if s is None:
                continue
            lines.append(
                f"| {c.hosts:>5} | {c.network_label:<7} | {tool:<7} "
                f"| {fmt_secs(s['median']):<7} | {fmt_secs(s['p95']):<7} "
                f"| {fmt_secs(s['min']):<7} | {fmt_secs(s['max']):<7} |"
            )
    lines.append("")
    return lines


def render_report(cells: list[CellResult]) -> str:
    cells = sorted(cells, key=lambda c: c.sort_key)
    md = cells[0].metadata
    earliest_ts = min(c.ts for c in cells)

    pve_lines = []
    for k, label in (("pve_cpu", "CPU"),
                     ("pve_cores", "Cores"),
                     ("pve_mem", "RAM"),
                     ("pve_storage", "Storage"),
                     ("pve_kernel", "Kernel"),
                     ("pve_version", "PVE")):
        if k in md:
            pve_lines.append(f"  - {label}: {md[k]}")

    out = [
        f"# Bench matrix — {earliest_ts}",
        "",
        "## Run metadata",
        "",
        f"- **scampi:** `{md.get('scampi', '?')}`",
        f"- **ansible:** `{md.get('ansible', '?')}`",
        f"- **hyperfine:** `{md.get('hyperfine', '?')}`",
        f"- **pve host:** {md.get('pve_host', '?')} ({md.get('pve_node', '?')})",
    ]
    out.extend(pve_lines)
    out.extend([
        f"- **runs per cell:** {md.get('runs', '?')} (cold + warm × scampi + ansible)",
        f"- **cells:** {len(cells)} "
        f"(hosts: {sorted({c.hosts for c in cells})}, "
        f"networks: {sorted({c.network_label for c in cells})})",
        "",
        "Per-cell hyperfine output is in `*.{cold,warm}.{scampi,ansible}.{json,md}`",
        "and `*.metadata.txt`. This file is the consolidated read.",
        "",
        "## Summary",
        "",
    ])
    out.extend(render_phase_table(cells, "cold"))
    out.extend(render_phase_table(cells, "warm"))
    out.append("")
    out.append("## Distribution detail")
    out.append("")
    out.extend(render_detail_table(cells, "cold"))
    out.extend(render_detail_table(cells, "warm"))
    return "\n".join(out)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    src = ap.add_mutually_exclusive_group(required=True)
    src.add_argument("--since", type=Path,
                     help="marker file; aggregate metadata files newer than it")
    src.add_argument("--glob", type=str,
                     help="glob pattern relative to bench/results/")
    src.add_argument("metadata_files", nargs="*", type=Path, default=[],
                     help="explicit list of metadata.txt paths")
    ap.add_argument("--results-dir", type=Path, default=Path("bench/results"),
                    help="results dir (for --since / --glob)")
    ap.add_argument("--out", type=Path,
                    help="output path (default: <earliest-ts>.report.md "
                         "in --results-dir)")
    args = ap.parse_args()

    # collect candidate metadata files
    if args.metadata_files:
        candidates = list(args.metadata_files)
    elif args.since:
        marker_mtime = args.since.stat().st_mtime
        candidates = sorted(
            p for p in args.results_dir.glob("*.metadata.txt")
            if p.stat().st_mtime > marker_mtime
        )
    else:
        candidates = sorted(args.results_dir.glob(args.glob))

    cells = []
    for p in candidates:
        if not CELL_RE.match(p.name):
            print(f"skip (not a matrix cell): {p}", file=sys.stderr)
            continue
        try:
            cells.append(load_cell(p))
        except (FileNotFoundError, json.JSONDecodeError, KeyError) as e:
            print(f"skip {p}: {e}", file=sys.stderr)

    if not cells:
        print("no matrix cells found", file=sys.stderr)
        return 1

    report = render_report(cells)
    earliest = min(c.ts for c in cells)
    out_path = args.out or args.results_dir / f"{earliest}.report.md"
    out_path.write_text(report)
    print(f"wrote {out_path} ({len(cells)} cells)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
