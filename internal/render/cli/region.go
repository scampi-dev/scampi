// SPDX-License-Identifier: GPL-3.0-only

// The ephemeral live region: lane lines, overflow, N-of-M footer.

package cli

import (
	"fmt"
	"time"

	"scampi.dev/scampi/internal/render/ansi"
	"scampi.dev/scampi/internal/render/layout"
)

// regionLines formats the ephemeral live region: per deploy lane, up to 3
// longest-running in-flight steps (spinner, tag, index/kind/desc, elapsed), then
// "(+N more)" past the cap. Returns nil off-TTY (no region there).
func (c *CLI) regionLines(f *inflight, frame int) []string {
	if !c.isTTY {
		return nil
	}
	c.refreshGeometry()
	const maxPerLane = 3
	spin := ""
	if n := len(c.glyphs.spinner); n > 0 {
		spin = c.glyphs.spinner[frame%n]
	}
	now := c.now()
	var lines []string
	for _, lv := range f.view() {
		shown, extra := lv.Running, 0
		if len(shown) > maxPerLane {
			extra = len(shown) - maxPerLane
			shown = shown[:maxPerLane]
		}
		for _, r := range shown {
			// Spinner stays default/uncolored: the step is still in flight, so
			// coloring it a verdict color (yellow "changed" etc.) would prejudge an
			// outcome that hasn't happened yet.
			line := "  " + spin +
				c.deployTag(r.ref.Deploy) +
				c.formatter.fmtfMsg(colStepKind, " [%d]%s", displayIndex(r.ref.Index), kindSuffix(r.ref.Kind)) +
				c.descSuffix(r.ref.Desc, colStepDesc) +
				c.formatter.fmtfMsg(colOpDesc, "  (%s)", now.Sub(r.started).Truncate(time.Second))
			lines = append(lines, c.finalizeRegion(line))
		}
		if extra > 0 {
			lines = append(lines, c.finalizeRegion("  "+c.formatter.fmtfMsg(colOpDesc, "(+%d more)", extra)))
		}
	}
	// N-of-M progress footer: finished steps so far against the run-wide plan
	// total. Hook steps sit outside the plan total, so they get a separate
	// "+N hooks" suffix instead of overrunning it.
	var footer string
	if done, total, hooks := f.progress(); total > 0 {
		label := fmt.Sprintf("%d/%d steps", done, total)
		if hooks > 0 {
			label += fmt.Sprintf(" +%d hook%s", hooks, layout.Plural(hooks))
		}
		footer = c.finalizeRegion("  " + c.formatter.fmtMsg(colOpDesc, label))
	}
	// Cap the region below the terminal height: a region taller than the screen
	// breaks CursorUp (it clamps at the top row), so scrolled-off lines can't be
	// erased and every repaint would spam scrollback. Lane lines get cut, the
	// footer always survives.
	if maxLines := c.height - 1; c.height > 0 && len(lines) >= maxLines {
		if footer != "" {
			maxLines--
		}
		lines = lines[:max(maxLines, 1)]
	}
	if footer != "" {
		lines = append(lines, footer)
	}
	return lines
}

// finalizeRegion applies the same redact + width-fit + reset that durable lines
// get in commitRenderEvents, since region lines bypass that path.
func (c *CLI) finalizeRegion(line string) string {
	line = c.formatter.redact(line)
	line = layout.FitLine(line, c.width, c.glyphs.ellipsis)
	if c.shouldUseColor() {
		line += ansi.Reset
	}
	return line
}
