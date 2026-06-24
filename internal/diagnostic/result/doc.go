// SPDX-License-Identifier: GPL-3.0-only

// Package result holds the computed value types of the one-shot surface:
// the structures engine commands return and the CLI prints once - execution
// reports, plan trees, inspect details. It is the sibling of event/, which
// holds the stream surface. Neither package imports engine, so render can
// consume both without coupling to the engine.
package result
