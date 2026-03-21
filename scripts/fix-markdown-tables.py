#!/usr/bin/env python3
"""Fix markdown table alignment so all rows have the same width.

Pads each cell so pipe characters align vertically. Preserves separator
row alignment markers (:---:, :---, ---:) and applies matching alignment
(left, center, right) to content cells.
"""

import re
import subprocess
from pathlib import Path

TABLE_ROW = re.compile(r"^\|.*\|$")


def is_table_row(line: str) -> bool:
    return bool(TABLE_ROW.match(line.rstrip()))


def split_cells(row: str) -> list[str]:
    return [c.strip() for c in row.strip().strip("|").split("|")]


def is_separator_row(cells: list[str]) -> bool:
    return all(re.match(r":?-+:?$", c.strip()) for c in cells)


def cell_alignment(sep: str) -> str:
    """Return 'left', 'center', or 'right' from a separator cell."""
    sep = sep.strip()
    left = sep.startswith(":")
    right = sep.endswith(":")
    if left and right:
        return "center"
    if right:
        return "right"
    return "left"


def align_cell(text: str, width: int, alignment: str) -> str:
    if alignment == "center":
        return text.center(width)
    if alignment == "right":
        return text.rjust(width)
    return text.ljust(width)


def sep_cell(old: str, width: int) -> str:
    """Rebuild a separator cell (e.g. :---:) to the given width."""
    old = old.strip()
    left = old.startswith(":")
    right = old.endswith(":")
    dashes = width - (1 if left else 0) - (1 if right else 0)
    if dashes < 1:
        dashes = 1
    return (":" if left else "") + "-" * dashes + (":" if right else "")


def format_table(rows: list[str]) -> list[str]:
    parsed = [split_cells(r) for r in rows]
    ncols = max(len(r) for r in parsed)

    for r in parsed:
        while len(r) < ncols:
            r.append("")

    sep_idx = 1 if len(parsed) > 1 and is_separator_row(parsed[1]) else None

    # Determine alignment per column from separator row
    alignments = ["left"] * ncols
    if sep_idx is not None:
        for j, cell in enumerate(parsed[sep_idx]):
            alignments[j] = cell_alignment(cell)

    # Compute max width per column (excluding separator)
    widths = [0] * ncols
    for i, cells in enumerate(parsed):
        if i == sep_idx:
            continue
        for j, cell in enumerate(cells):
            widths[j] = max(widths[j], len(cell))

    result = []
    for i, cells in enumerate(parsed):
        parts = []
        for j, cell in enumerate(cells):
            w = widths[j]
            if i == sep_idx:
                parts.append(" " + sep_cell(cell, w) + " ")
            else:
                parts.append(" " + align_cell(cell, w, alignments[j]) + " ")
        result.append("|" + "|".join(parts) + "|")

    return result


def fix_file(path: Path) -> bool:
    lines = path.read_text().splitlines()
    out = []
    i = 0
    changed = False

    while i < len(lines):
        if is_table_row(lines[i]):
            table_rows = []
            while i < len(lines) and is_table_row(lines[i]):
                table_rows.append(lines[i])
                i += 1
            if len(table_rows) >= 2:
                fixed = format_table(table_rows)
                if fixed != table_rows:
                    changed = True
                out.extend(fixed)
            else:
                out.extend(table_rows)
        else:
            out.append(lines[i])
            i += 1

    if changed:
        path.write_text("\n".join(out) + "\n")
    return changed


def main():
    root = Path(__file__).resolve().parent.parent

    result = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "*.md"],
        capture_output=True, text=True, cwd=root,
    )
    files = [root / f for f in result.stdout.splitlines()]

    fixed = 0
    for f in sorted(set(files)):
        if fix_file(f):
            print(f"fixed: {f.relative_to(root)}")
            fixed += 1

    if fixed:
        print(f"\n{fixed} file(s) fixed")
    else:
        print("all tables aligned")


if __name__ == "__main__":
    main()
