#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
#
# Regenerate the site's Nerd Font subset from internal/render/cli/glyph.go.
#
# glyph.go is the single source of truth for CLI glyphs (see CLAUDE.md).
# The site embeds terminal transcripts containing those glyphs, so it needs
# a font carrying exactly them - no more (bytes) and no less (tofu).
#
# Two artifacts are generated, both committed:
#
#   site/data/glyphs.json              codepoints, consumed by the CSS template
#   site/static/fonts/<subset>.woff2   the font itself
#
# Test_Rule_FontSubset fails if glyphs.json drifts from glyph.go, so a new
# glyph cannot reach the site without the font being rebuilt.
#
# Everything is pinned: the Nerd Fonts release by tag + sha256, fonttools by
# version, uv by .mise.toml. A fresh checkout reproduces the same bytes.
set -euo pipefail

NERD_VERSION="v3.5.1"
NERD_SHA256="01172f37db8543edb102e5cb5c64101c9f4686630804d49b419aa07b23a69996"
NERD_FACE="SymbolsNerdFontMono-Regular.ttf"
FONTTOOLS_VERSION="4.62.1"

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

glyph_src="internal/render/cli/glyph.go"
data_out="site/data/glyphs.json"
font_out="site/static/fonts/nerd-symbols-subset.woff2"

codepoints=$(./scripts/glyph-codepoints.py "$glyph_src")
echo "codepoints: $codepoints"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Cache the upstream archive under XDG so a regen is offline once warm.
# Validation is against the pinned sha256, so a cache hit needs no network
# and a corrupted or tampered cache entry is refetched rather than trusted.
# Keyed by version: bumping NERD_VERSION misses the cache by construction.
cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/scampi/nerd-fonts/${NERD_VERSION}"
archive="$cache_dir/NerdFontsSymbolsOnly.tar.xz"
mkdir -p "$cache_dir"

sha_of() { [[ -f "$1" ]] && shasum -a 256 "$1" | awk '{print $1}'; }

if [[ "$(sha_of "$archive")" == "$NERD_SHA256" ]]; then
  echo "using cached nerd-fonts $NERD_VERSION"
else
  echo "fetching nerd-fonts $NERD_VERSION"
  curl -fsSL -o "$archive.tmp" \
    "https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_VERSION}/NerdFontsSymbolsOnly.tar.xz"

  actual=$(sha_of "$archive.tmp")
  if [[ "$actual" != "$NERD_SHA256" ]]; then
    rm -f "$archive.tmp"
    echo "error: checksum mismatch for NerdFontsSymbolsOnly.tar.xz" >&2
    echo "  want: $NERD_SHA256" >&2
    echo "  got:  $actual" >&2
    exit 1
  fi
  # Only publish into the cache once verified, so an interrupted download
  # never leaves a half-file that looks like a valid entry.
  mv "$archive.tmp" "$archive"
fi

tar -xJf "$archive" -C "$work" "$NERD_FACE"

mkdir -p "$(dirname "$font_out")" "$(dirname "$data_out")"
uv run --quiet --with "fonttools[woff]==${FONTTOOLS_VERSION}" \
  pyftsubset "$work/$NERD_FACE" \
  --unicodes="$codepoints" \
  --flavor=woff2 \
  --output-file="$font_out"

# The CSS unicode-range is templated from this, so it can never disagree
# with the font that was actually built. The sha256 lets Test_Rule_FontSubset
# verify the committed woff2 too -- without it a stale font would pass the
# gate as long as the codepoint list happened to look right.
font_sha=$(shasum -a 256 "$font_out" | awk '{print $1}')
cat > "$data_out" <<JSON
{
  "unicodeRange": "${codepoints//,/, }",
  "fontSha256": "$font_sha"
}
JSON

echo "wrote $font_out ($(wc -c < "$font_out" | tr -d ' ') bytes)"
echo "wrote $data_out"
