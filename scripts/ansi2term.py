#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
"""Convert scampi's ANSI-colored output into the site's <div class="term"> HTML.

The landing page (site/content/about.md) hand-embeds terminal transcripts as
<span class="...">-tagged HTML so they render with the semantic palette in that
page's <style> block. Rather than transcribe those by hand (and drift from the
real CLI), pipe live output through this filter:

    scampi plan  -v --color=always demo.scampi | scripts/ansi2term.py
    scampi check -v --color=always demo.scampi | scripts/ansi2term.py

It reads ANSI on stdin and writes the coalesced span markup on stdout. Only the
SGR codes scampi actually emits are mapped, onto the class names about.md
defines (m/mb plan, c step, d detail, y change, g correct, r failure). Trailing
blank lines are dropped. Substitutions like a machine-specific owner belong in
the caller (e.g. `| sed 's/pskry:staff/root:root/'`), not here.
"""
import re
import sys

# SGR code -> semantic class, matching the .term palette in about.md.
CLASS = {
    "35;2": "m", "35;1": "mb",  # plan rail / plan header
    "36": "c", "36;1": "c",     # step boundaries
    "90;2": "d", "90": "d",     # detail / dim
    "33": "y",                  # change / mutation
    "32;2": "g", "32": "g",     # correctness / stability
    "31": "r", "31;1": "r",     # failure
}

SGR = re.compile(r"\x1b\[([0-9;]*)m")


def esc(s):
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def convert(text):
    out = []
    for line in text.split("\n"):
        # Split the line into (text, code) runs on each SGR escape.
        last, segs = 0, []
        for m in SGR.finditer(line):
            segs.append((line[last:m.start()], m.group(1)))
            last = m.end()
        segs.append((line[last:], None))
        # Assign a class to each text run, coalescing adjacent same-class runs.
        runs, cls = [], None
        for txt, code in segs:
            if txt:
                if runs and runs[-1][0] == cls:
                    runs[-1] = (cls, runs[-1][1] + txt)
                else:
                    runs.append((cls, txt))
            if code is not None:
                cls = None if code in ("", "0") else CLASS.get(code, cls)
        out.append("".join(
            esc(t) if c is None else f'<span class="{c}">{esc(t)}</span>'
            for c, t in runs
        ))
    while out and out[-1].strip() == "":
        out.pop()
    return "\n".join(out)


if __name__ == "__main__":
    sys.stdout.write(convert(sys.stdin.read()))
