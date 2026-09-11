#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only

"""Print the Nerd Font codepoints used by the CLI glyph set.

internal/render/cli/glyph.go is the single source of truth for CLI glyphs
(CLAUDE.md). The site embeds terminal transcripts containing them, so its
font subset is generated from this list rather than hand-maintained.

Nerd Font icons live in supplementary private use area A (U+F0000+). Box
drawing and braille (the spinner frames) are ordinary codepoints that every
monospace font already carries, so they are deliberately excluded.

Output is a comma-separated unicode-range list: U+F0026,U+F012C,...
"""

import sys

NERD_PUA_START = 0xF0000


def main() -> None:
    path = sys.argv[1] if len(sys.argv) > 1 else "internal/render/cli/glyph.go"
    with open(path, encoding="utf-8") as fh:
        found = {ord(c) for c in fh.read() if ord(c) >= NERD_PUA_START}

    if not found:
        print(f"error: no Nerd Font codepoints in {path}", file=sys.stderr)
        raise SystemExit(1)

    print(",".join(f"U+{c:04X}" for c in sorted(found)))


if __name__ == "__main__":
    main()
