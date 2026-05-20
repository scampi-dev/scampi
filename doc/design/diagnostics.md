# Diagnostics

The diagnostic system grew when there was nothing real to surface, so
the default became "log everything we can think of". That defaulted us
into a wide lifecycle surface (engine started, plan started, action
started, op checked, hook fired, etc.) carrying severity gradations
and chattiness levels to make the noise manageable.

The actual signal users want is much smaller: **is my config valid,
where are the issues, what would/did change, am I making progress, and
how did the run end.** Everything else is decoration.

This document captures the philosophy behind the surface. Concrete
type names and method signatures drift; they live in `diagnostic/`
and `diagnostic/event/` and can be read directly.

## Two families: streaming vs return values

The system emits **two distinct families**: streaming signals that
fire during work, and command outputs that are the one-shot answer to
a specific command. Only the first family rides the emit pipeline.

### Streaming signals (event pipeline)

1. **Diagnostics** — "your config / environment is wrong here, here's
   why, here's how to fix it". Carries source span, severity, hint,
   help.
2. **Changes** — "this op would change X" (planned) or "this op did
   change X" (executed). Drift detection output, apply-time mutation
   reports.
3. **Progress** — "currently connecting to host:foo", "currently
   checking action 3/12: posix.user(alice)". A status line; latest-
   wins on TTY, appended one line per update on non-TTY.

### Command outputs (return values, NOT events)

These are the actual answer of a command invocation. They have no
severity, no cause, no streaming semantics — they're one-shot per
invocation, returned directly from engine entry points, and the
calling `cmd/scampi` handler renders them via a dedicated renderer.

- **Plan output** — returned from `Engine.Plan`. Per-deploy action
  plans + cross-deploy topology in one shape.
- **Inspect output** — returned from `Engine.InspectList`. Resolved
  field values per step.
- **Index output** — returned from `Engine.IndexAll` /
  `Engine.IndexStep`. Step catalog and per-step documentation.
- **Summary** — end-of-run one-liner computed by the CLI from the
  `ExecutionReport` returned by `Engine.Check` / `Engine.Apply`. Not
  an event; not its own return type; just something the cmd renders
  after work finishes.

The split exists because command outputs are the entire reason the
user invoked scampi. Routing them through a streaming pipeline buys
nothing and forces every consumer (CLI, future JSON, LSP) to
reconstruct what is already known statically.

## What dies (kept for historical context)

The pre-rewrite system had a lifecycle event for every phase
boundary — engine started, plan started, step planned, action
started, op check started, op execute finished, etc. Each event
carried a severity field and a "chattiness" field so the renderer
could be told "shut up about this one". Every emitter implementation
had a per-kind method.

That's all gone. Diagnostics don't have lifecycle; lifecycle has no
useful signal. Per-action and per-op chatter is filler that exists
only because we used to emit it. Without lifecycle, severity and
chattiness collapse: severity is what diagnostic-kind already encodes
(Error vs Warning vs Info), and there's no chatter to dial down.

The Emitter interface collapsed accordingly: one `Emit(Event)` plus
one `Raise(Raisable)` helper for the error-producer case. Sealed
union of concrete event types (Error, Warning, Info, Change,
Progress) is the discriminator.

## Why a sealed Event union, not an interface zoo

Each concrete event type encodes its severity by type, not by a
field. `event.Error` aborts via its `Impact` field when the producer
asks it to; `event.Warning` and `event.Info` never abort. This means:

- Producers' `Diagnostic()` method body is the proof that they
  return the right severity (code review catches mismatches).
- Renderer dispatch is a switch on concrete type, not a switch on a
  field.
- Policy can flip Warning → Error by replacing the event type, not
  by mutating a field (the `WarningsAsErrors` knob).

The interface is sealed via an unexported `isEvent()` method on each
type, so external code can't accidentally extend the union.

## Cause: optional context tag

Most events have no notable trigger; hooks are the first thing that
does. The `Cause` field is a value type with the zero value meaning
"no notable trigger", so it costs nothing to carry.

When code enters a hook context it wraps the emitter via
`WithCause(...)`. The wrap stamps the Cause on every event passing
through, unless the event already has a non-zero Cause (explicit
wins over wrapper). Future Cause kinds — retry context, deferred
resource arrival, scheduled re-eval — get added the same way and
become rendering decisions, not new emission paths.

## Ordering and threading

Every event has a `Time` field set at emit. The CLI renders events
in emission order. Concurrent emits (op pool, plan workers) serialize
at the displayer boundary — single mutex around the output write, or
a buffered channel feeding one writer. Implementation detail.

Emission order is the guarantee; wall-clock order isn't. Two ops
emitting at nearly the same instant may serialize in either order.
This matches today's behavior and is the right tradeoff (lock-free
producer side).

## LSP consumer

The LSP server constructs a buffering Emitter for each evaluation
pass. The linker raises every diagnostic through it. After the pass
completes, the LSP iterates the buffered events and translates each
to a `protocol.Diagnostic` for the editor. Change and Progress
events that reach the LSP are ignored — neither is a file
diagnostic.

Plan-phase diagnostics (cycle detection, missing producers, etc.)
reach the LSP because the linker walks those checks without
executing targets, and they route through the same Emitter as
authoring-phase diagnostics. This is the "killer LSP" payoff.
