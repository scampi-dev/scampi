#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only

import os
import sys

# Chars that smell of AI/markdown-rendered prose. Add to this set as
# new offenders surface. Glyph fallbacks in render/cli/glyph.go are
# the only sanctioned non-ASCII; everything else is noise.
CHARS = {
    "\u2014": "em-dash",
    "\u2013": "en-dash",
    "\u2026": "ellipsis",
    "\u2192": "right-arrow",
    "\u2190": "left-arrow",
    "\u2191": "up-arrow",
    "\u2193": "down-arrow",
    "\u21d2": "rightwards-double-arrow",
    "\u21d0": "leftwards-double-arrow",
    "\u2022": "bullet",
    "\u00b7": "middle-dot",
    "\u201c": "left-double-quote",
    "\u201d": "right-double-quote",
    "\u2018": "left-single-quote",
    "\u2019": "right-single-quote",
    "\u00a0": "nbsp",
    "\u2009": "thin-space",
    "\u200b": "zero-width-space",
    "\u00d7": "multiplication-sign",
    "\u2713": "check-mark",
    "\u2717": "ballot-x",
    "\u2705": "white-heavy-check",
    "\u274c": "cross-mark",
    "\u2728": "sparkles",
}

EXTS = {
    ".go",
    ".md",
    ".scampi",
    ".sh",
    ".py",
    ".lua",
    ".toml",
    ".yaml",
    ".yml",
    ".just",
    ".justfile",
}
SKIP_DIRS = {
    ".git",
    "build",
    "node_modules",
    ".sandbox",
    "vendor",
    ".tree-sitter-scampi",
    ".tree-sitter-scampi-mod",
    ".nvim-treesitter",
    ".infra",
    ".modules",
}
# Files where unicode is sanctioned.
SKIP_FILES = {
    "render/cli/glyph.go",
}

hits = 0
for root, dirs, files in os.walk("."):
    dirs[:] = [d for d in dirs if d not in SKIP_DIRS and not d.startswith(".")]
    for name in files:
        path = os.path.join(root, name)
        rel = path.lstrip("./")
        if rel in SKIP_FILES:
            continue
        _, ext = os.path.splitext(name)
        if ext not in EXTS and name not in ("justfile", "Justfile"):
            continue
        try:
            with open(path, encoding="utf-8") as fp:
                for i, line in enumerate(fp, 1):
                    for c in line:
                        if c in CHARS:
                            hits += 1
                            print(f"{path}:{i}: {CHARS[c]} ({c!r}): {line.rstrip()}")
                            break
        except (UnicodeDecodeError, OSError):
            continue

if hits:
    print(f"\n{hits} suspicious unicode hit(s)", file=sys.stderr)
    sys.exit(1)
print("clean", file=sys.stderr)
