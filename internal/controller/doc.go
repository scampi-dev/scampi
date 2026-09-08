// SPDX-License-Identifier: GPL-3.0-only

// Package controller defines the machine scampi runs on, as an I/O surface:
// configs, templates, secrets, environment, and the local artifact cache.
//
// Controller-side operations read config inputs and write to the local cache
// (downloaded files, inline content). They are distinct from target-side
// operations, which perform convergence mutations - even when both sides are
// the same machine.
package controller
