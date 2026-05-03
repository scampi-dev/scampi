---
title: Why scampi exists
description: The pain that drove me to build scampi, and the architectural bet underneath.
---

## The pain

A while back I had a UniFi fleet I wanted to manage like code. The plan was straightforward: write an Ansible collection, ship a few modules (`network`, `device`, `port_config`), set up integration tests, done. Standard playbook stuff. I did all of that. Here's what it looked like by the time I gave up.

The collection had a role inside it called `dsl/`. That was my YAML task library for orchestrating Wiremock during integration tests — starting it, stopping it, stubbing the UniFi login, asserting that exactly one `POST` went out. I was building a DSL inside YAML to escape YAML. That sentence is the whole origin story in one line.

Some of what was in that DSL role:

- Wiremock got started by `nohup java -jar wiremock-standalone-3.13.2.jar &` — inside an `ansible.builtin.shell` task. Its readiness was polled with `wait_for` and `uri` retries. The test runner was orchestrating a JVM from YAML.
- Every integration target (`tests/integration/targets/network/`, `.../port_config/`) was a literal `### Arrange / ### Act / ### Assert` block, with `ansible.builtin.import_role` calls between each step into the DSL. Readable. Ceremonial. Slow.
- The Python module `network.py` defined the same API surface three times — once as Ansible `argument_spec`, once as a `desired` dict, once as the JSON the UniFi controller actually wanted. Drift between the three was a constant source of bugs that the type system couldn't catch, because there was no type system.

Then on top of all that, the daily authoring experience.

`ansible-language-server` is slow on every keystroke, and it's missing features I'd expect from a modern LSP. Try renaming a role: there's no symbol-aware rename, just text search-and-replace and a prayer. The mandatory collection scaffolding — paths like `ansible_collections/<ns>/<name>/plugins/modules/`, nested five levels deep before you write any code, with `roles/`, `tests/integration/targets/`, `module_utils/` all enforced by Ansible's discovery — means breaking convention silently makes Ansible unable to find your code, with no useful error. Getting completions for an in-tree module you're *currently writing* requires `ANSIBLE_COLLECTIONS_PATH` juggling or a `--force` reinstall ritual. And `ansible-lint` fights you on perfectly fine YAML with rules that feel like opinion-as-policy.

Some of this pain was self-inflicted. I had decided to do it *right* — arrange/act/assert, mocked API, request-count assertions, idempotency verification — and the tooling couldn't meet that standard without me hand-rolling a DSL inside it.

The pattern I kept hitting: I was the compiler in a stack where every layer was YAML pretending to be a programming language, and every test was an interpreter shipping JVM bytecode through shell tasks. The 90-second iteration loops for one-line changes were the symptom. The architecture was the disease.

## What I wanted

So I sat down and wrote what would make this not suck. Short list:

- **A real language with a real type system.** One source of truth for any API surface. One place to fix a bug.
- **A single static binary on both sides.** No Python on the target. No plugin matrix. No `ansible-galaxy install -r`.
- **Tests in milliseconds**, in-process, with mocks as a first-class primitive. I never wanted to download a JAR and `nohup java -jar` it again.
- **An LSP that runs the *real* evaluator** — what I see in the editor is what runs at apply time. Rename, find-references, goto-def, the lot.
- **Errors that walk me to a valid config** without me opening the docs. Point at *my* source, show the fix using *my* values, surface the next problem only after I've fixed this one.
- **And the one I didn't see at the time, but it ended up running everything:** primitives that compose against any environment, with no transport treated as the One True Way.

Then I started writing the thing I wished existed.

## The bet

Two commitments shape every other decision in scampi. I won't pretend I sat down and architected them — they fell out of taking that list seriously. But once you commit to those two, the rest of the design has nowhere else to go.

**One: it's a real language with a real type system, and the same evaluator runs everywhere.** Engine and `scampls` (the LSP) share one pipeline. Not a separate syntax pass for the editor; the actual evaluator with the full step registry. Unknown fields, missing required params, type mismatches, invalid enums, unresolved imports, capability mismatches between a step and the target it'll run against — all caught as you type, with source spans pointing at *your* code and Hints showing the fix using *your* values. What you see in the editor is what the engine would produce at runtime — they're literally the same code.

**Two: targets and steps are orthogonal, with typed contracts between them.** A target advertises capabilities (filesystem, file mode, ownership, packages, services, commands…). An op declares which capabilities it needs. The engine matches them at plan time. There is no "default mode". There is no privileged transport. `local` is not a stripped-down fallback. `ssh` is not the real one. `rest` is not a step. They're peer environments, and the entire step library composes against any target satisfying the right contract.

The second one is the part most people don't see at first, and it's the part that pays for the rest of the architecture.

## What that gets you

Once those two are settled, a bunch of things you'd usually have to engineer separately come for free:

- **Single binary, single tool.** scampi is one Go binary; `scampls` is its own. Static, no runtime dependencies, no agents, no daemon. For SSH targets nothing needs installing on the far end either — scampi opens a single multiplexed session and runs plain commands you can read in `ps`.
- **Errors that hold your hand.** Every diagnostic carries a typed ID, a source span pointing at the offending construct, and a Hint that shows the fix using your own values. Reach a valid config by following errors. This is a guiding principle, not a feature checkbox — every new error path is held to it.
- **No state file. Target is truth.** Every check inspects live state; every apply re-checks before mutating. No remote backend to lock, no plan/apply divergence, no drift blindness. `scampi check` after a manual change shows you the drift instantly, because there's no cached idea of what reality should look like to disagree with reality itself.
- **Drift detection on every check, not a separate mode.** `check` is `apply` with mutations stripped. Same plan, same connection, same evaluations.
- **Tests in milliseconds.** Mock targets are in-process — `test.target.in_memory()` for POSIX, `test.target.rest_mock()` for HTTP APIs (records every request for assertion). No Docker, no JVM, no playback layer. Write a `*_test.scampi`, assert on the outcome, done.
- **Files, not scaffolding.** `.scampi` files live wherever you organise them; the module system uses imports, not directory conventions. No five-level discovery rules to satisfy.
- **Batteries included.** Step libraries for POSIX, REST, containers, and PVE ship with the binary. No plugin install, no external registry, no version-pin-and-pray.
- **Parallel where it can be, sequential where it must be.** Within an action, ops form a DAG and run concurrently as their declared inputs and outputs allow. Within a deploy block, action ordering falls out of those same declarations — independent actions parallelize, dependent ones serialize, no human-curated `notify` / `when` lists involved.
- **Source and target are different worlds.** Secrets are resolved, templates rendered, downloads cached on the source side. The target never sees your vault. That's a security property of the architecture, not a convention.

## Proof one: UniFi as a user-space module

The case that started this whole thing — UniFi — is the cleanest demonstration of the architecture, because *none of the UniFi support is in the binary*. It lives at `scampi.dev/modules/unifi`, written as pure scampi, imported via the module system the same way you'd `go get` a Go package.

**Layer one: bindings, generated.** The full UniFi REST surface — both the modern Integration API and the legacy controller — is mechanically generated from UniFi's OpenAPI specs by `scampi gen api`. Around 13,500 lines of thin `rest.request` wrappers across the two APIs, regenerable whenever the upstream spec changes. No waiting on a maintainer to add module support. No drift between the wrappers and the spec.

**Layer two: resources, hand-composed.** Around 175 lines of hand-written scampi turn those generated wrappers into idempotent declarations. Each public type — `unifi.network`, `unifi.client`, `unifi.client_set` — is built from a `rest.resource { query, missing, found, bindings, state }` block. `rest.resource` is a first-class scampi primitive: declare the desired state, the engine queries the API, diffs against reality, and converges. **Idempotency is the primitive's responsibility, not yours.** That's why hand-composition is short — you describe shape, not lifecycle.

**Layer three: the user.** Imports the module, points at a controller, declares what should be true:

```scampi
module main

import "std"
import "std/rest"
import "scampi.dev/modules/unifi"

let controller = rest.target {
  name     = "unifi"
  base_url = "https://10.10.1.1/proxy/network"
  auth     = rest.header { name = "X-API-Key", value = "..." }
  tls      = rest.tls_insecure {}
}

std.deploy(name = "lan", targets = [controller]) {
  unifi.client {
    site = "default"
    name = "pihole"
    mac  = "aa:bb:cc:dd:ee:ff"
    ip   = "10.10.10.10"
  }
}
```

Drift detection, idempotent apply, parallel ops — all from the engine, all free. None of it required Go code in the scampi binary. The same problem that drove me to build this language is now ~13,500 lines of generated bindings and ~175 lines of resource composition, all pure scampi, all in user-space. **The same recipe works for any HTTP API**: Cloudflare, GitHub, the Proxmox API, your internal control plane.

## Proof two: PVE — provision and configure in one run

UniFi proves the REST family. PVE proves something stranger: the same step library composes across *transports it was never written for*, as long as the target satisfies the contract.

scampi handles a Proxmox host in two phases, both in the same file.

**Provisioning.** First target is an SSH host pointing at the PVE node. `pve.lxc` runs against it, talks to the Proxmox API, and creates the container from scratch — storage, networks, mount points, SSH keys injected into `authorized_keys`, the lot.

**Configuration.** Second target is `pve.lxc_target` for the container that just came up. It SSHes to the PVE host and tunnels every operation through `pct exec <vmid>` into the LXC. Crucially, this target *satisfies the POSIX contract* — which means the entire `posix.*` library works against it unchanged. `posix.pkg`, `posix.copy`, `posix.template`, `posix.service`, `posix.user` all run inside the freshly provisioned container without knowing they're being multiplexed through `pct exec` on the host. The step library doesn't know it's tunneling. The target does the multiplexing.

```scampi
module main

import "std"
import "std/posix"
import "std/ssh"
import "std/pve"

let pve_host = ssh.target     { name = "pve", host = "10.0.0.5", user = "root" }
let box      = pve.lxc_target { name = "box", host = "10.0.0.5", user = "root", vmid = 200 }

// Provision: create the LXC on the PVE host.
std.deploy(name = "create-box", targets = [pve_host]) {
  pve.lxc {
    id       = 200
    node     = "pve"
    hostname = "box"
    memory   = "1G"
    networks = [pve.LxcNet { name = "eth0", bridge = "vmbr0", ip = "10.0.0.20/24" }]
  }
}

// Configure: same posix.* steps you'd run anywhere — through `pct exec` here.
std.deploy(name = "configure-box", targets = [box]) {
  posix.pkg     { packages = ["nginx"], source = posix.pkg_system {} }
  posix.service { name = "nginx", state = posix.ServiceState.running, enabled = true }
}
```

One language. One step library. One file. Two targets — the Proxmox API for "make the box exist", and the pct-exec multiplexer for "make the box do its job". No handoff to a second tool, no glue scripts between phases, drift detection at every layer. The provisioning/configuration split that other tools enforce turns out to be an artifact of *their* architecture, not a real boundary.

The recipe generalizes. A `redfish.target` for OOBM and bare-metal provisioning (iLO, iDRAC, modern IPMI all speak Redfish) would slot in the same way: a step creates the target by powering on the host and mounting install media; subsequent steps deploy to it once it's up. Cloud APIs (`aws.target`, `gcp.project`) become REST families with auth and pagination lifted into the target. `k8s.cluster` for declarative cluster operations. None need a special slot in the engine — they just need to satisfy a capability contract.

## Where it stops

If you were wondering why everything in this story revolves around POSIX — it's not because Windows is impossible in this model, it's because I won't be the one to write it. A `winrm.target` and a Windows step family with ACL-aware semantics would slot into the engine the same way `ssh.target` and `posix.*` do today. That's a contribution to scampi proper — Go code in the binary, since these are core target and step types, not a user-space module like UniFi — but the door is wide open. If you know Windows the way I know POSIX: come write it. The water's warm.
