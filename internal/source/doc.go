// SPDX-License-Identifier: GPL-3.0-only

// Package source defines source-side access: configs, templates, secrets,
// environment, and the local artifact cache.
//
// Source-side operations read configs and write to the local cache (downloaded
// files, inline content). They are distinct from target-side operations, which
// perform convergence mutations — even when both sides are the same machine.
package source
