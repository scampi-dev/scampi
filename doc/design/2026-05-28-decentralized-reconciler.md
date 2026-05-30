# scampi: Decentralized Reconciler for Bare Metal

**Status:** Working spec, 2026-05-28. Captures the design conversation
between pskry and Claude. Locked decisions are stated as facts; open
questions are called out explicitly in the Open Questions section.

## TL;DR

A decentralized, eventually consistent reconciler for bare-metal
infrastructure. Same statically-linked binary runs everywhere as a peer;
the fleet IS the cluster. Resources are CRD-shaped declarations with
implicit dependencies inferred from references, reconciled continuously
against live observation of reality.

Niche: everything K8s doesn't reach -- bare metal, hypervisors, OS config,
K8s day-0 bootstrap, dumb network gear -- delivered with cloud-native
ergonomics.

Tagline: "lightweight K8s for bare metal", or more honestly: "malware that
fixes your infra".

## Motivation

The K8s reconciliation pattern -- declarative desired state + controllers +
continuous convergence -- is the most successful ops idea of the last
decade. Nothing in the ops landscape applies it end-to-end to
non-container infrastructure. Crossplane and ClusterAPI come closest but
are K8s-coupled.

Real whitespace exists for:

- Reconciler-based config without K8s overhead.
- A language people don't hate (Puppet/Chef were reconcilers; their DSLs
  were loathed).
- Heterogeneous targets: hosts that can run agents AND dumb gear that can't.
- Self-bootstrapping fleets from a single seed.

## Non-goals

- Workload orchestration (K8s, Nomad).
- In-cluster GitOps (Flux, Argo).
- Replacing NixOS for users happy with NixOS-the-whole-stack.
- Cloud-resource provisioning at scale (Terraform / Pulumi own that).

## Scope (the niche)

scampi manages:

- Bare metal: PXE, BMC, IPMI.
- Hypervisors: Proxmox, KVM, ESXi.
- OS-level configuration: users, packages, services, files, firewall.
- K8s day-0 bootstrap: cluster formation, CNI install (Cilium), kube-vip,
  the critical bits before ArgoCD can take over.
- Dumb network gear: switches, firewalls, BMCs, Unifi controllers --
  anything API/SSH-reachable that can't host an agent.

Hands off to:

- K8s for workloads.
- ArgoCD / Flux for in-cluster lifecycle.

## Architecture overview

Decentralized peer mesh. No central control plane.

- Same statically-linked binary everywhere. Multiple modes:
  - `scampi run` -- long-running peer daemon.
  - `scampi apply` -- one-shot from anywhere (day-0 infection).
  - `scampi status`, `scampi get`, `scampi describe` -- query the mesh.
- Every peer:
  - Joins a gossip mesh (hashicorp/memberlist or equivalent).
  - Reads desired state from a configured source.
  - Reconciles the resources it's responsible for.
  - Exposes `/healthz`, `/readyz`, `/metrics` on a configurable interface.
  - Holds a small local action log; gossips entries to peers for failover.
- The fleet is the cluster. No designated leader, no quorum nodes.
  Per-resource leases (gossip-based) handle ownership of work that can't
  be locally executed.
- No Raft. scampi's state model (stateless observation + tiny action log
  + git as desired state) doesn't require strong consistency. Eventual
  consistency with idempotent ops is enough.

### Bootstrap: the infection model

A scampi seed is `binary + config`. The config points at a source (git URL
+ auth, a local directory, an HTTP URL, or content embedded in the
binary). Day 0: drop the seed on a host with network access, run
`scampi apply`, it starts walking the graph.

There is no "bootstrap mode" in the binary. The day-1 invocation on a
laptop and the day-N peer running on managed infra are the same code
path.

### The seed is not a controller

The "seed" is a role a peer plays for a few minutes during cut-over, not
a machine that exists forever. As soon as the bootstrapped peers join the
gossip mesh (>= 2 peers with appropriate reach), the original seed is
fungible. It can leave.

Seed shapes:

- **Temporary seed**: laptop, NUC, USB-baked ISO. Brings up a rack, then
  unplugs. Common for greenfield rack-and-stack.
- **Permanent seed**: an ops workstation that lives in the system long-
  term (monitor + keyboard, browser for Grafana / Cockpit). It happens to
  be the first peer AND stays as a regular mesh member. Most push-only
  work naturally settles there because it has the broadest label set, but
  blades can take over via lease re-election if the workstation is down.

There is no `seed-mode` flag, no seed Kind. There's just `scampi run` on
a peer that has reach to the gear it needs to bootstrap.

## Resource model

Three layers, one model:

1. **Primitives** -- Go controllers compiled into the binary. The atoms
   that touch reality:
   `host`, `network`, `package`, `file`, `service`, `user`, `group`,
   `command`, `http_target`, `ssh_target`, `secret`, `git_ref`, `cert`, ...
2. **Built-in composed Kinds** -- scampi-language files shipped in the
   binary's stdlib. The value-add:
   `samba_dc`, `pve_node`, `pve_cluster`, `k3s_cluster`, `unifi_network`,
   `certbot`, `kube_vip`, ...
3. **User-defined Kinds** -- scampi-language files in the user's source.
   Their abstractions.

All three use the same Kind / Resource / Controller model. A controller
for a composed Kind creates Resources of other Kinds -- no separate
composition language, no "compositions are special" gotcha. K8s pattern:
Pod is built-in, Deployment is built-in but composes ReplicaSet; user
CRDs are the same shape.

### Implicit dependencies

Dependencies are inferred from references. No `depends_on` for the common
case:

```
network "internal" { vlan = 10 }

host "nuc-1" {
  mac     = "aa:bb:cc:dd:ee:ff"
  network = network.internal                 # implicit dep
}

pve_cluster "main" {
  nodes = [host.nuc-1, host.nuc-2]           # implicit deps
  vip   = "10.0.0.5"
}

samba_dc "ad" {
  host   = pve_cluster.main.vm("samba-dc")   # implicit dep on VM up
  domain = "lan.example.com"
  users  = git("users/").as_ldap()
}

certbot "samba-cert" {
  for = samba_dc.ad.fqdn                     # implicit dep on samba up
}
```

`depends_on` remains as an escape hatch for the ~5% of ambient
dependencies that aren't reference-based (e.g., "k3s cluster needs the
network to be routable, but nothing in k3s's config references the
network").

### Composition everywhere: every value is a deferred expression

Field values are not raw scalars -- they are expressions that can be
literals, references, computations, or external lookups:

```
samba_dc "ad" {
  domain    = "lan.example.com"                          # literal
  admin_pw  = vault("scampi/samba/admin")                # external lookup
  users     = git("users/").as_ldap()                    # git read + transform
  fqdn      = "ad." + self.domain                        # computed
  bind_to   = first(host_ips(host = self.host, network = "internal"))
}
```

Every value is potentially "not-yet-known" (Pulumi-style Outputs /
promises). The reconciler waits on unresolved references. This is
surprisingly invasive in the type system but is the right model.

The Kind catalog has built-in producers for common sources: `git`,
`vault`, `http.get`, `env`, `file`, plus refs to other resources. New
producers are pluggable.

## State model

- **Desired state:** lives in the configured source. Two peers reading
  the same source see the same desired state.
- **Observed state:** never cached. Every reconciliation loop re-queries
  reality for what it needs. Within a single loop, memoize for
  performance; across loops, throw it away.
- **Action log:** the only persistent state. Append-only, TTL'd. Stores
  in-flight actions, recently completed actions, backoff timers. NOT
  observed state. NOT output values. Tiny -- a small fleet produces
  kilobytes. Local to each peer; gossip-replicated for failover of
  cross-peer work.

Consequences:

- Drift is impossible to hide. The reconciler cannot believe a stale
  cache. Sysadmin pokes? Next loop sees it.
- HA is trivial. Stateless peers + tiny replicable action log.
- "Migrate central from laptop to infra" is a no-op. Nothing to migrate.

## Source of declarations

The desired-state source is an abstraction:

```
source "local" { path = "./config" }
source "git"   { url = "git@github.com:me/infra.git", ref = "main" }
source "http"  { url = "https://infra.example.com/state.tar" }
source "embed" { }   # baked into the binary, USB / ISO scenario
```

Each implementation handles its own caching, change detection, and
refresh cadence. Same `List() / Read() / Hash() / Watch()` interface
upstream. Same code path for all.

Ship with `local` and `git`. `http` and `embed` come later.

Fleet consistency: in a multi-peer fleet, all peers must point at the
same source. Git is the recommended fleet source because every peer
naturally pulls the same ref. Local dirs are for testing, day-0
bootstrap, and air-gap scenarios.

## Ownership and placement

Each resource has a placement expression with two knobs,
K8s-affinity-inspired:

- **`require`** -- hard filter. Peers that don't match are not eligible.
- **`prefer`** -- soft ranking. All matching peers are eligible;
  preferred ones are picked first.

Default placement per resource shape:

- **Host-tied resources**:
  `prefer { instance == self.host }; require { has-route-and-creds }`.
  Local when possible, fleet fallback when not.
- **Cross-host work**:
  `require { has-route-to-all-participants }`.
- **External resources**:
  `require { has-route-and-creds }; elected via gossip lease`.
- **Sensitive resources** can opt into local-only:
  `require { instance == self.host }; no fallback`. Fails loud if the
  local peer is down.

Defaults Just Work. Placement expressions are an escape hatch for the
rare cases that need different semantics.

### Lease re-evaluation

Leases are not held until death. When a new peer joins with a higher
preference score, the current owner gracefully hands off. This is what
keeps cheap workloads on cheap iron -- if dnsmasq is preferred on the
ops workstation and the workstation comes back from a reboot, the blade
that took over should hand the lease back.

Re-evaluation is bounded by a `placement_stability` debounce window
(default: a few minutes) so peer churn doesn't cause flapping. Exact
mechanism (hand-off vs. break-and-reelect, debounce default) is an open
question -- see Open Questions.

### Failure visibility

When peer X is unreachable, `up{job="scampi", instance="X"} == 0` and
Alertmanager fires. scampi does not reinvent Prometheus. Peer-down is
detected by the standard observability stack; lease takeover is
invisible to alerting because the resource gets reconciled anyway.

## Reconciliation cadence

Polling with jitter:

- Each Kind declares a default poll interval (`host`: 60s, `service`:
  30s, `network`: 5m, etc.).
- Per-resource override allowed.
- +/-10% jitter per peer so simultaneous-tick storms don't hammer
  external APIs.
- Reconciliation IS observation. Observe, compute diff, act if needed.

Event-driven triggers (webhooks, file watches, agent push) are a later
add. Polling is enough for the bare-metal world where most things don't
change every second.

## Drift behavior

Two orthogonal axes.

### What to do when drift is observed

Per-resource `drift_policy` field:

```
firewall_rule "prod_block_external" {
  drift_policy = "revert"   # default: re-converge silently
}

samba_dc "ad" {
  drift_policy = "alert"    # emit event, don't act
}

ad_user "ceo" {
  drift_policy = "halt"     # stop reconciling this resource pending ack
}
```

Default = revert. Sensitive things opt into alert or halt.

### What counts as drift (scope over collections)

Resources that manage a collection of things declare scope:

```
samba_dc "ad" {
  users      = git("users/").as_ldap()
  user_scope = "additive"   # declared users must exist; extras tolerated
  # vs
  user_scope = "exclusive"  # only declared users exist; extras deleted
}
```

Default = additive. Exclusive is opt-in for collections where you want
canonical control. Get this default wrong and you delete production users
on day 1.

## Security

### Peer comms

Transport is pluggable, but the *protocol* (what messages peers exchange)
is fixed. Ship with gRPC; a second transport (REST?) gets added only
when someone needs it.

Security level configurable per deployment:

- Insecure -- localhost only, dev.
- PSK -- same-LAN trust.
- TLS -- cross-LAN.
- mTLS -- production.

Per-peer config in the seed. scampi enforces what you ask for, doesn't
force a level on you.

### Secrets

Secrets are a Kind with a pluggable `source` backend:

```
secret "samba_admin_pw" {
  source = sops("secrets/samba.yaml", key = "admin_pw")
}

samba_dc "ad" {
  admin_pw = secret.samba_admin_pw
}
```

Ship with sops + age. Vault, env, file, http, KMS, TPM backends slot in
over time.

### Secret zero

How does the age key reach each peer? Deferred to day-0 operational
design. For now: place manually at provisioning time, or bake into the OS
image. The user's Mac holds the canonical age key initially.

## Versioning and upgrades

- Signed, statically-linked binaries.
- Semver.
- No side-loading. All Kinds (primitives and composed) ship in the binary
  or are loaded from the source.
- Self-update modeled as a Kind:
  `scampi_release { version = "1.4.2", channel = "stable" }`. Each peer
  reconciles itself toward the target. Atomic swap + restart.
- Protocol compat policy: peers at versions N and N+1 always talk; N and
  N+2 not guaranteed. Forces orderly rolling upgrades.
- Rolling upgrade, one peer at a time, abort on first failure. No canary
  in the initial release.
- Binary distribution: GitHub Releases default, configurable mirror for
  corp / air-gap envs.

## Observability

scampi does not reinvent the stack. It exposes:

- Prometheus metrics on `/metrics` -- domain metrics like
  `scampi_resource_drift_detected_total`,
  `scampi_reconcile_duration_seconds`,
  `scampi_lease_holder{resource}`.
- Health on `/healthz` and `/readyz`.
- Structured logs (JSON); optional OTEL traces.

Whatever needs alerting is alerted via standard Alertmanager rules. Peer
down? `up{} == 0`. Drift? Domain metric. No scampi-specific dashboards or
alerting subsystem to maintain.

## Language

**Open call.** Proposed: HCL.

User's instinct was YAML to avoid inventing a language. Counter-proposal
is HCL because the "composition everywhere" principle requires
expressions inside values from day 0, which YAML cannot do natively. HCL
gives expressions and references natively, reads ~80% like YAML, and
Terraform has proven it works at this shape. We get the off-the-shelf
benefit without the YAML + templating tax.

The language WILL evolve. Inventing one from scratch now is the wrong
place to spend energy.

## Open questions

- **Language:** HCL vs alternatives (YAML, KCL, CUE, Nickel, Dhall,
  custom). YAML rejected (no native expressions; "composition everywhere"
  requires expressions inside values from day 0). HCL leading; pending
  decision.
- **Placement affinity grammar:** exact syntax for `require` + `prefer`.
  K8s-inspired but doesn't have to be K8s-identical.
- **Lease re-evaluation policy:** hand-off vs. break-and-reelect when a
  higher-preference peer joins. Debounce window default.
- **Event-driven reconciliation triggers:** initial-release stretch or
  later?
- **Day-0 bootstrap operational design:** USB seed format, ISO bake, etc.
  Deferred.
- **Multi-tenancy / RBAC.** Not discussed.
- **Schema migration of Kinds** across scampi versions. Not discussed.
- **Cross-resource transactions** (e.g., "join host X to AD atomically").
  Not discussed.
- **Backup / DR of the action log.** Not discussed.
- **First-release milestone definition:** what's the minimum demo that
  proves the model? Not discussed.
