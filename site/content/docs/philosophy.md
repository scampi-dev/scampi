---
title: Philosophy
weight: 10
---

scampi is an opinionated tool. This page explains what those opinions are
and why they exist.

## Convergence, not scripting

Most infrastructure automation is imperative: run these commands, in this
order, and hope the starting state is what you expected. If someone
manually tweaked a config file or a previous run half-failed, you're in
uncharted territory.

scampi is a convergence engine. You declare the desired end state. On
every run, scampi compares reality to your declaration and makes the
minimum changes needed to close the gap. If there's no gap, nothing
happens.

This means:

- **Runs are safe to repeat.** After a reboot, after a deploy, after
  someone made a manual change — `scampi apply` always brings the system
  back.
- **Partial failures are recoverable.** If a run is interrupted, the next
  run picks up where it left off. There's no corrupted state to untangle.
- **Drift is visible.** `scampi check` shows you exactly what has drifted
  from your declared state without changing anything.

## A real language, not YAML

YAML is not a programming language. It's a data serialization format
pressed into service as a configuration language, and it shows — type
coercion surprises (`"no"` becomes `false`), no variables, no functions,
no way to reduce duplication except copy-paste or a templating layer
bolted on top.

If you've ever had to loop over included YAML files just to repeat a
task with different parameters — writing the task in one file, the loop
in another, the variables in a third — you know the feeling. That's not
a design flaw in any specific tool. It's what inevitably happens when a
data format gets pressed into service as a programming language: control
flow gets bolted on sideways, and you end up fighting the format instead
of describing your infrastructure.

Stateful IaC tools have a similar story. They solved real problems —
tracking what exists, knowing what to destroy — but the state file
becomes its own source of drift, its own failure mode, its own thing to
manage. Convergence doesn't need external state because the target *is*
the state.

Every tool in this space made reasonable trade-offs for their time and
context. scampi just starts from a different premise: if the
configuration language is powerful enough, you don't need the workarounds.

scampi has its own configuration language. It's:

- **Statically typed.** Types, enums, attributes, optionals — all checked
  at link time. Errors surface as you type, not when you run.
- **Declarative-first.** Decl calls (`posix.copy { src = …, dest = … }`)
  are struct literals, not function calls dressed up as configuration.
  The syntax matches the intent.
- **Composable.** Modules, imports, user-defined `func`s and `decl`s,
  comprehensions, conditionals, loops. You can abstract, compose, and
  reuse the way you would in code, because it *is* code.
- **Familiar.** If you've read Python, HCL, or Bicep, you'll be reading
  scampi configs in five minutes.

You never need a templating engine to generate your config. Your config
is the program.

When you *do* need templates — for config files deployed to targets —
scampi uses Go's `text/template`. It's battle-tested, expressive, and
runs in-process with no external dependencies. No separate template
runtime, no version mismatches between your template engine and your
config tool.

## Batteries included, not plugin sprawl

The plugin ecosystem approach to IaC is well-established: thousands of
community modules and providers, each maintained separately. The upside is
breadth. The downside is that they're variably documented, occasionally
abandoned, and sometimes subtly incompatible with each other or with new
versions of the core tool.

scampi takes the opposite approach: everything you need for system
convergence is built in. Packages, files, templates, services, containers,
firewall rules, users, groups, mounts, REST APIs — they all ship with the
binary.

This isn't because extensibility is bad. It's because:

- **Consistency matters.** Every built-in step follows the same patterns,
  the same error conventions, the same check/execute contract. They work
  together because they were designed together.
- **Versioning is simple.** One binary version, one set of capabilities.
  No dependency matrix, no "this module requires core >= 2.3 but < 3.0".
- **Discovery is trivial.** `scampi index` lists everything. The
  docs cover everything. There's no hunting across repos and registries.

The `run` step exists for genuinely niche needs — masking a systemd unit,
toggling a specific firewalld zone, disabling tmpfs on a particular mount.
Things where a dedicated step would be over-engineered because the
command is the right abstraction. That's not the answer for "scampi
doesn't support X yet."

The answer for that is: contribute it. scampi is open source, and new
steps are welcome — but they land in *this* codebase, not in an external
registry. There's no plugin system, no downloading modules at runtime, no
hoping that `scampi-community/fancy-step` is still maintained in two
years. If a step is useful enough to exist, it's useful enough to be
reviewed, tested, and shipped with the binary. That's how you get
consistency, that's how you get documentation, and that's how you avoid
the graveyard of abandoned third-party modules that plagues every plugin
ecosystem.

Batteries included doesn't mean "we thought of everything." It means
everything lives in one place.

## Fail fast, fail clearly

scampi validates everything it can before touching the target system. Type
errors, missing fields, capability mismatches, unresolvable references —
these are all caught during planning, not halfway through execution.

When something does fail, the error message is designed to get you to a
working config without reading documentation:

- **It says what's wrong.** Not "invalid value" — what value, why it's
  invalid, what was expected.
- **It shows the fix.** A concrete example of correct syntax, using the
  values you already wrote, not generic placeholders.
- **It guides progressively.** Fix one error, get the next. Each
  correction reveals the next issue until your config is valid.

This is a core design principle, not polish. The error experience *is* the
user experience for a config tool — it's the main thing you interact with
while iterating on a config.

## Testable by default

Infrastructure code is notoriously hard to test. Ansible has Molecule —
a separate tool with its own config format, its own Docker driver, and
its own learning curve. Terraform has `terraform plan` and that's about
it. Most people's test suite for their infrastructure is "apply it and
see if anything breaks."

scampi treats testing as a first-class concern, not an afterthought bolted
on by a third party. Test files are regular scampi configs that run against
mock targets:

```scampi {filename="webserver_test.scampi"}
module main

import "std"
import "std/posix"
import "std/test"
import "std/test/matchers"

let mock = test.target_in_memory(
  name    = "mock",
  initial = test.InitialState { packages = ["nginx"] },
  expect  = test.ExpectedState {
    files    = {"/etc/nginx/sites-enabled/example.com.conf": matchers.has_substring("server_name example.com")}
    services = {"nginx": matchers.has_svc_status(posix.ServiceState.running)}
  },
)

std.deploy(name = "test", targets = [mock]) {
  // ... steps that configure nginx for example.com
}
```

No containers to spin up, no cloud accounts to provision, no YAML
scaffolding. `scampi test` runs in milliseconds because mock targets are
in-memory — there's no I/O, no network, no waiting. REST API modules
get the same treatment with a REST mock target, which records every HTTP
request for assertion verification.

This isn't a nice-to-have. If you can't test your infrastructure code
without deploying it, you don't iterate — you gamble. Fast, local,
deterministic tests are how infrastructure code becomes actual
engineering instead of "push and pray."

## Developer ergonomics matter

The gap between writing infrastructure code and getting feedback on it
should be as small as possible. Long feedback loops breed sloppy configs
and manual workarounds.

scampi ships `scampls`, a Language Server Protocol server that runs the
real evaluation pipeline in your editor. Not a lint pass, not a syntax
check — the actual scampi evaluator with the full step registry. You
get unknown-field errors, missing-required-field errors, type mismatches,
and invalid enum values as you type, with precise source spans and
actionable hints.

`scampi gen api` bridges external systems: point it at an OpenAPI spec
and it generates typed scampi wrappers for every endpoint. The
generated functions compose directly with `rest.resource` for idempotent
CRUD — no manual HTTP plumbing, no copy-pasting URLs.

Error messages are designed so you can reach a valid config by just
following them. Each error says what's wrong, shows correct syntax using
your values, and guides you to the next fix. The error experience *is*
the developer experience for a config tool — optimizing it isn't polish,
it's the whole point.

## Source and target are separate worlds

scampi draws a hard line between two sides:

- **Source side**: where scampi runs, where your configs live, where
  templates are rendered, where secrets are resolved, where downloads are
  cached.
- **Target side**: where mutations happen — the system being converged.

With `local.target { … }`, both sides are the same machine. With
`ssh.target { … }`, they're different machines. Either way, the engine
treats them as separate concerns.

This separation means:

- **The target never talks to your secret store.** Secret resolution
  and template rendering happen source-side. The target receives rendered
  files — it never needs credentials to your vault or access to your
  secret backend.
- **Source content is composable.** A `copy` step doesn't care if its
  content comes from a local file, an inline string, or a URL. The source
  resolver handles that independently.
- **Targets are capability-based.** A target advertises what it can do
  (filesystem, packages, services). Steps declare what they need. If there's
  a mismatch, scampi tells you at plan time, not after it's halfway through
  applying changes.

## Check before execute, always

Every operation in scampi implements two methods: check and execute.

**Check** inspects current state and reports whether a change is needed.
**Execute** makes the change — and only runs if check says it's necessary.

This is the foundation of idempotency. It's not a suggestion or a best
practice — it's enforced by the type system. You literally cannot add an
operation to scampi without implementing both sides.

`scampi check` is not a special mode. It runs the real engine — same
planning, same target connection, same checks against real system state.
The only difference is that it stops before the mutation step. There's no
separate "dry run" simulation that might disagree with what apply actually
does. Check *is* the first half of apply.

Steps also declare what they produce and consume — a `dir` step promises
the path it creates, a `copy` step declares it needs that path to exist.
During check, if an earlier step *would* create something a later step
depends on, scampi knows the dependency will be satisfied at apply time
and doesn't flag it as an error. This means check gives you an accurate
picture even when your steps have ordering dependencies — no false
positives from a resource that doesn't exist *yet* but will by the time
it's needed.

## Opinions over options

scampi doesn't try to be all things to all people. Some choices are baked
in:

- **Parallel where possible, sequential where necessary.** Steps declare
  what they produce and consume. Independent steps run in parallel
  automatically; steps with dependencies are ordered by the engine. Within
  a single step, ops form a DAG and run concurrently where their
  dependencies allow. You get parallelism without giving up determinism.
- **Targets are an abstraction.** `local.target { … }` converges the
  machine you're sitting at. `ssh.target { … }` converges a remote host.
  `rest.target { … }` converges a REST API. The engine doesn't care — it
  sees capabilities, not transports. A target doesn't have to be a
  machine. Local is a first-class citizen, not a consolation prize for
  when you don't have an inventory file. Managing your own workstation
  is a perfectly valid use case.
- **Transparent on the target.** Every command scampi runs — local or
  remote — is a plain command visible in process lists and auditable in
  logs. Nothing opaque gets shipped and executed in a blob. For remote
  targets, scampi opens a single SSH session and multiplexes commands
  over it. There's no per-step overhead of establishing a new
  connection, booting an interpreter, and tearing it all down — only to
  do it again for the next step. Operations are small and cheap because
  the transport is already there. This is also why scampi can afford to
  do proper check-before-execute on every op without it feeling slow.
- **One config language.** scampi everywhere. Not "YAML for simple
  cases, a real language for complex ones" — that split always creates
  two ecosystems that don't compose.
- **Single binary.** No runtime dependencies, no interpreters to install,
  no virtual environments. Copy it to a machine and it runs. For remote
  targets, nothing needs to be installed on the other end either — just
  SSH and a POSIX shell.
- **Colored, semantic output.** Yellow means change, green means already
  correct, red means failure. Not configurable, because consistent meaning
  matters more than aesthetic preference.

These aren't limitations — they're the decisions that keep the tool simple
enough to fit in your head.
