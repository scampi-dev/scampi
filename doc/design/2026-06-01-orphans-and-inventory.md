# Orphan Handling and Inventory

**Status:** Working design, 2026-06-01. Captures the design conversation
between pskry and Claude. Locked decisions stated as facts; future-work
clearly marked.

Companion to [`2026-05-28-decentralized-reconciler.md`](2026-05-28-decentralized-reconciler.md).
Slots into the State model section and refines reconciliation behavior.
Does not alter the broader architecture.

## TL;DR

Orphans (resources removed from the declared snapshot that scampi
previously applied) get auto-destroyed by default. The set of managed
refs lives as a projection over the action log: `apply.success` adds an
entry, `destroy.success` removes it. The action log remains the single
persistent state.

Phase A (this slice): single-node, full replay on startup, no
compaction. Phases B and C add snapshot+truncate and mesh coordination
respectively, when log size or mesh shape forces them.

## Background

The main spec describes two adjacent axes for change handling. Neither
is implemented yet; both predate this design and remain on the roadmap:

- `drift_policy`: per-resource policy for what to do when observed
  state diverges from a managed declaration. Values: `revert`, `alert`,
  `halt`.
- Collection scope (`additive` / `exclusive`): how to treat collection
  members not enumerated in a parent resource.

Neither axis covers what happens when a top-level declaration
disappears entirely between snapshots. That is the question this
design answers.

## Problem statement

Two snapshots taken one tick apart:

```
snapshot N:    dir "lol" { path = "/tmp/lol" }   other resources...
snapshot N+1:  other resources...                (no dir "lol")
```

What does scampi do at tick N+1 for `/tmp/lol`?

Four broad framings exist:

- **Auto-destroy.** Terraform-with-state, K8s with `--prune`. The
  declarative source is canonical; removed declaration means "should
  not exist".
- **Auto-preserve.** Ansible default, Terraform-without-state. Removed
  declaration means "I no longer assert anything"; the resource stays.
- **Mixed by Kind.** Atomic / cheap Kinds destroy on removal; heavy /
  expensive Kinds preserve. Per-resource overrides.
- **Halt.** Refuse to act, warn loudly, wait for operator ack.

scampi picks **auto-destroy**, uniformly, no per-Kind variation in v1.

## Tracking model

Detecting orphans requires knowing what scampi managed at the time the
previous snapshot was applied. That requires persistence somewhere.

Three candidate forms were considered:

- **scampi-side state file.** Small file in scampi's data dir listing
  managed refs and their identifying attrs. Cleanest as a standalone
  mechanism, but introduces a third persistent thing alongside config
  and action log.
- **Breadcrumb in the world.** Apply marks the resource with a
  "managed by scampi" tag (xattr, marker file, systemd drop-in,
  iptables comment). Each Kind invents its own scheme. Per-Kind
  feasibility varies wildly. Vulnerable to operator deletion of the
  mark. Loop-time discovery (e.g. `find / -name .scampi`) is
  prohibitively expensive.
- **Projection over the action log.** Inventory derived by replaying
  log events. Action log already exists, already destined for gossip
  replication per the main spec, no new persistent thing.

Decision: **projection over the action log.** The action log was
always going to be the persistence story; extending its role to "source
of truth for what scampi manages" reuses the existing primitive
instead of inventing a new one.

## Inventory

The in-memory inventory is a `map[Ref]ManagedEntry` derived by
replaying the log:

```
apply.success     -> inventory[ref] = {kind, name, identifying-attrs}
destroy.success   -> delete inventory[ref]
apply.failed      -> no change
destroy.failed    -> no change
log.*             -> no change
```

`apply.success` and `destroy.success` MUST carry the identifying
attributes needed to later destroy the resource: `path` for `file` and
`dir`, the package name for `package`, etc. Replay reads those from the
event.

Properties:

- Replay is pure in-memory folding. Idempotent.
- Same log, same inventory. Deterministic across nodes.
- A `destroy.success` neutralizes the prior `apply.success`. Once
  destroyed, scampi has no memory of the resource. No tombstone, no
  future reaction.
- New nodes catch up by replaying the log.

scampi's view of the world is bounded by the union of the declared set
and the inventory set. Anything outside that union is invisible by
design. If a sysadmin creates `/tmp/lol` after scampi destroyed it and
forgot it, scampi will not touch it.

## Reconciliation algorithm

Each reconcile tick:

```
declared        = parse(snapshot_dir)
inventory       = current in-memory projection
apply_targets   = declared
destroy_targets = inventory - declared

for r in destroy_targets:
    try Kind(r).Destroy()
    on success: emit destroy.success, inventory -= r
    on failure: emit destroy.failed, inventory unchanged (orphan stays)

for r in apply_targets:
    try Kind(r).Apply()
    on success: emit apply.success, inventory += r
    on failure: emit apply.failed, inventory unchanged
```

`Apply()` on a resource that already exists in sync is a no-op that
still emits `apply.success` so the inventory tracks it. This is
K8s-style adoption: declaring a resource effectively adopts any
matching reality.

Destroy ordering is the reverse of apply ordering (reverse topo-sort
over the inventory's recorded dependencies). Phase A only has `file`
and `dir`, which have no cross-Kind references in their destroy path,
so reverse ordering is not yet exercised. The mechanism lands when the
first cross-Kind dependency demands it.

## Action log

### Location

Per node, local on disk. Defaults:

- System install (root): `/var/lib/scampi/actionlog/`
- User mode: `$XDG_STATE_HOME/scampi/actionlog/` (so
  `~/.local/state/scampi/actionlog/` by default)

Overridable with `--action-log <dir>` on the CLI. The existing flag
today takes a file path; this design widens it to take a directory.

### Format

Directory of `.jsonl` segment files: `0001.jsonl`, `0002.jsonl`, and so
on. Each segment is append-only, capped at ~50MB (configurable). When
the active segment hits the cap, scampi rotates: closes it, opens a new
one with the next sequence number, continues appending.

Phase A only ever has one segment in practice (no truncation, no
forced cap). Rotation activates fully in phase B alongside the
snapshot+truncate machinery.

Each line is one JSON object:

```
{
  "ts":   "2026-06-01T21:45:45Z",
  "code": "apply.success",
  "ref":  "dir.lol",
  "path": "/tmp/lol"
}
```

Fields:

- `ts`: RFC3339 timestamp.
- `code`: event code (see below).
- `ref`: `<kind>.<name>` of the resource. Omitted for non-resource
  events.
- Additional keys depend on the event code. For `apply.success` /
  `destroy.success` on `file` / `dir`, this is `path`. Other Kinds
  add whatever their `Destroy` method needs.

### Event codes

Existing:

- `snapshot.received`, `snapshot.rejected`
- `apply.start`, `apply.success`, `apply.failed`
- `log.debug`, `log.info`, `log.warn`, `log.error` (filtered out of the
  action log; only emitted via slog)

New in this design:

- `destroy.start`, `destroy.success`, `destroy.failed`: parallel shape
  to the `apply.*` codes.
- `inventory.snapshot` (phase B+): single event whose payload is a
  full inventory dump. See Phasing.

## Per-Kind destroy semantics (v1)

- `file.Destroy`: `os.Remove(path)`. Errors propagate.
- `dir.Destroy`: `os.Remove(path)`. Refuses to remove a non-empty
  directory and returns the standard error. That error becomes
  `destroy.failed`; the orphan stays in the inventory and the operator
  sees a loud warning. Recursive deletion is opt-in only and not
  provided in v1.

`file.Destroy` does not need to refuse on "non-empty" since files do
not have that property.

## Adoption (future work)

Phase A: scampi always adopts. If an `Apply` finds the resource already
exists and matches the declaration, it noops and still emits
`apply.success`. The inventory tracks it from that point.

Risk: auto-destroy + silent adoption means "declaring a resource that
happens to already exist, then later removing the declaration" will
destroy reality scampi did not create. The escape hatch is the future
per-resource opt-out described below.

### Per-resource adopt field (deferred)

Field name TBD (candidates: `adopt`, `adopt_existing`,
`auto_adopt_existing`). Default `true`. When `false`:

- If reality already has the resource at first apply: halt this
  resource. Warn. Do not apply. Do not add to inventory. Other
  resources continue.
- The resource stays in `declared, not-in-inventory,
  real-thing-exists` purgatory until the operator resolves it: delete
  the real thing, set `adopt = true`, or remove the declaration.

This is parallel in shape to `drift_policy = halt`. Not implemented in
v1 because v0 resources are MVP and their attribute surface is not
being extended yet.

## Phasing

### Phase A (this design, immediate)

- Add `destroy.start`, `destroy.success`, `destroy.failed` event codes.
- Add a `Destroy` method to the Kind interface, implemented for `file`
  and `dir`.
- Move action log to the segmented-directory layout with the default
  paths above.
- Replay log on startup to build the in-memory inventory.
- Wire orphan detection into the reconcile loop: destroy first, then
  apply.
- Always adopt.

No snapshots, no truncation. Action log grows monotonically. Sufficient
for v1 single-node; gives months of headroom on small fleets before
size becomes a problem.

### Phase B (deferred)

Single-node compaction. Periodically write an `inventory.snapshot`
event containing a full dump of the current inventory. Atomic write
(one JSON line). When a snapshot exists, frozen segments wholly older
than the snapshot can be deleted.

Two snapshots are kept in the log at any time (current and previous) so
a hypothetical reader mid-replay from the previous one survives a
fresh truncation.

### Phase C (deferred, mesh)

Multiple peers. Each has its own local log. The log gossips for
failover per the main spec.

Snapshot writing becomes a leased role: one peer is elected to write
the periodic snapshot, others fold it into their local logs via
gossip. Catch-up: a joining peer fetches the latest snapshot from the
mesh and replays forward.

Catch-up safety: joiners always start replay from a snapshot, never
from log position 0. If a newer snapshot appears mid-replay (because
compaction ran on another peer), the joiner discards its partial state
and adopts the newer one. Cheap because replay is idempotent in-memory
folding.

## Adjacent: dry-run

With auto-destroy as the default, a `scampi apply --dry-run` flag
becomes a meaningful safety net. Expected output shape:

```
Would apply:   file.config, dir.data
Would destroy: dir.old_lol
```

No side effects, no `apply.*` / `destroy.*` events emitted. Thin
wrapper around the reconcile loop.

Own slice, lands after orphan handling itself, not blocking on it.

## Open questions

- **Action log line format details.** Field ordering and key naming
  conventions could use a pass. Pin during phase A implementation.
- **Reverse-destroy ordering.** Mechanism not exercised by v1's Kinds.
  Likely shape: store deps in the inventory entry, walk in reverse
  topo order when picking destroy targets. Lands with the first
  cross-Kind destroy dependency.
- **Identifying-attr discovery.** Currently `apply.success` is emitted
  with hand-picked kvs by each `applyKind` function. As the catalog
  grows, a convention is needed for "which attrs constitute the
  destroy key" per Kind. Options: Kind-level metadata, primary-attr
  convention, explicit registration. Phase A: hardcoded per Kind, fix
  the pattern later.
- **Crash safety.** If scampi crashes between performing a side
  effect (creating `/tmp/lol`) and writing the log entry, the
  inventory will be wrong on next replay. Phase A: accept the window,
  rely on idempotent apply to converge on the next tick. Strong
  durability (WAL, fsync per entry) is future hardening.
- **Documenting the log schema.** scampi reads its own log; external
  tools may want to as well. JSONL schema documentation is probably
  worth shipping in v1.
