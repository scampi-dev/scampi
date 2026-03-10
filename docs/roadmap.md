# Roadmap: Real-World Infrastructure

This document lays out the path from "works for files and packages on SSH hosts"
to "can replace a production Ansible homelab." It's based on a real gap analysis
against a 25-host, 35-role Ansible project with databases, Docker apps, reverse
proxies, directory services, logging, and monitoring.

The goal isn't feature parity with Ansible. It's answering the question: **what
does scampi need so that a practitioner would choose it over Ansible for a new
project, and be able to migrate an existing one incrementally?**

---

## Progress

**Phase 1: Foundation**
- [x] Secrets (v1: `secret()` builtin, `file` backend)
- [x] User-controlled backend selection (`secrets()` builtin)
- [x] Encrypted file backend (`age`)
- [ ] Deploy block dependencies (`after=`)

**Phase 2: Targets**
- [ ] REST target + auth
- [ ] REST escape hatch steps

**Phase 3: Step Types**
- [ ] Docker container
- [ ] User/group
- [ ] Repository

**Phase 4: Orchestration**
- [ ] Handlers / post-change triggers
- [ ] Domain targets (NPM, Grafana, etc.)

---

## Where We Are

scampi can converge files, directories, symlinks, templates, packages, and
services on local and SSH targets. The `run()` escape hatch covers everything
else with degraded guarantees. The diagnostic system, the check/execute model,
the DAG scheduler, and the CLI output are solid.

What's missing is everything between "I can configure one machine" and "I can
stand up and orchestrate a fleet of interdependent services."

---

## The Gaps, in Dependency Order

Each gap is ordered by what it unblocks. Later items depend on earlier ones.

### 1. Secrets

**Status:** V1 implemented. Plumbing is in place, first backend works.

**What exists:**

- `secret("key")` builtin — resolves at eval time, returns a plain string
- `secret.Backend` interface — `Name() string`, `Lookup(key) (string, bool, error)`
- `file` backend — reads a flat JSON `secrets.json` next to the config
- `source.Source.LookupSecret()` — wired through all Source implementations
- `source.WithSecrets()` decorator — attaches a backend to any Source
- Diagnostic errors: `SecretError`, `SecretNotFoundError`, `SecretBackendError`
- Secret values never appear in diagnostic output — only key names

```python
# V1 API — secret() with implicit backend (secrets.json convention)
db_password = secret("postgres.admin.password")

template(
    content="password={{.pass}}",
    data={"values": {"pass": secret("db_pass")}},
    dest="/etc/app/config",
    perm="0600", owner="app", group="app",
)
```

**What's next — user-controlled backend selection:**

The V1 backend is implicit (convention: `secrets.json` next to config). This
should become a `secrets()` builtin that the user controls, with the file
convention as a fallback default:

```python
# Explicit backend selection — user decides
secrets(backend="file", path="secrets.json")
secrets(backend="bitwarden", vault="my-vault")
secrets(backend="vault", addr=env("VAULT_ADDR"))
secrets(backend=env("SCAMPI_SECRET_BACKEND", "file"))
```

This matters for the deferred-config use case: a build-time scampi run can
resolve secrets from an enterprise vault, re-encrypt them for the deploy-time
backend, and package two artifacts — the config bundle (with encrypted secrets
store) and the decryption key (delivered out-of-band). Build-time and
deploy-time are independent runs with independent backends. The config says
*what* secrets it needs, the backend decides *how* to provide them.

| Context | Backend | Auth |
|---------|---------|------|
| Local dev | `file` (secrets.json) | None, plaintext |
| CI runner / web UI | `bitwarden`, `1password` | API token via env var |
| Airgapped deploy | `age` | Key delivered out-of-band |
| Enterprise | `vault` | Token from env or instance metadata |

**Design principles (unchanged):**
- Secrets are resolved at eval time, before planning
- Secret values never appear in diagnostic output, plan display, or logs
- The secret store is explicit — no 22-level precedence chain
- Backend selection is the user's choice, not scampi's opinion

---

### 2. Deploy Block Dependencies

**Status:** Deploy blocks are independent. No ordering between them.

**The problem:** Real infrastructure has phases. You stand up a VM, then
configure what's inside it. You deploy a Docker container via SSH, then
configure it via its REST API. You provision a database server, then create
databases and users on it.

In Ansible, this is implicit — plays run top-to-bottom in a playbook, or you
manage it manually with tags and separate `make` targets (and suffer half-hour
no-op runs because the tool can't skip satisfied phases efficiently).

**Design:**

```python
deploy("npm-infra",
    targets=["npm_host"],
    steps=[ pkg(...), template(...), docker_container(...) ],
)

deploy("npm-config",
    targets=["npm_api"],
    after=["npm-infra"],
    steps=[ proxy_host(...), cert(...) ],
)
```

`after` declares dependency edges between deploy blocks. The engine builds a
DAG of deploy blocks (same pattern it already uses for ops within an action)
and walks it in order. Independent blocks can run in parallel. Cycles are
detected at plan time.

This is not a new concept — it's extending the existing dependency model one
level up. The planner already handles op-level DAGs. Deploy-block DAGs are the
same machinery with wider scope.

**What this unblocks:**
- Multi-phase provisioning (SSH bootstrap then REST configure)
- Cross-service dependencies (database must exist before app that uses it)
- Efficient re-runs (satisfied phases are checked and skipped, not re-executed)

---

### 3. REST Target

**Status:** Only SSH and local targets exist. No HTTP transport.

**The problem:** Half of real infrastructure management is API-driven. Reverse
proxies, monitoring systems, databases, container orchestrators, DNS services —
they all expose REST APIs for configuration. Ansible uses the `uri` module as a
generic escape hatch and builds modules on top of it. scampi needs the same
layered approach, but with the check/execute model baked in.

**Design — two layers:**

#### Layer 1: REST transport target

The base layer. Handles HTTP, authentication, serialization, headers, error
handling. Analogous to the SSH target handling shell transport.

```python
target.rest(
    name="npm_api",
    base_url="http://10.10.2.30:81/api",
    auth=rest.bearer(
        token_endpoint="/tokens",
        identity=secret("npm.admin.email"),
        secret=secret("npm.admin.password"),
    ),
)
```

The REST target provides a `RestClient` capability:

```go
type RestClient interface {
    Get(ctx context.Context, path string) (Response, error)
    Post(ctx context.Context, path string, body any) (Response, error)
    Put(ctx context.Context, path string, body any) (Response, error)
    Delete(ctx context.Context, path string) (Response, error)
}
```

Authentication strategies are pluggable at the target level:
- `rest.basic(user, password)` — HTTP Basic
- `rest.bearer(token_endpoint, identity, secret)` — OAuth/JWT token flow
- `rest.header(name, value)` — Static header (API keys)
- `rest.oauth2(...)` — Full OAuth2 client credentials

The target handles token lifecycle (acquire, cache, refresh) transparently.
Steps never see auth details.

#### Layer 2: REST escape hatch steps

Low-level steps for direct HTTP operations. The REST equivalent of `run()`.

```python
rest.request(
    desc="create proxy host",
    method="POST",
    path="/nginx/proxy-hosts",
    body={...},
    check=rest.expect(method="GET", path="/nginx/proxy-hosts",
                      jq=".[] | select(.domain_names[0] == \"npm.skrynet.dk\")"),
)
```

Or simpler — just the verb steps:

```python
rest.put(path="/users/1", body={...},
         check=rest.get(path="/users/1", expect={"email": "admin@skrynet.dk"}))
```

These preserve check/execute: `check` does a GET and compares, `execute` does
the mutation. Same contract as `run()` but over HTTP instead of shell.

#### Layer 3: Domain targets (future)

Higher-level targets that wrap REST and expose convergence-native capabilities.

```python
target.npm(
    name="npm_api",
    base_url="http://10.10.2.30:81/api",
    auth=rest.bearer(...),
)
```

An NPM target would implement `ProxyHostManager`, `CertManager` — domain
capabilities like `PkgManager` is for packages. Step types like
`npm.proxy_host()` would consume these capabilities.

This mirrors the SSH model exactly:

```
SSH target                    REST target
  └─ auto-detect backend        └─ explicit domain target
     ├─ apt/dnf/pacman             ├─ NPM: proxy hosts, certs
     ├─ systemd/openrc             ├─ Grafana: dashboards, orgs
     └─ capabilities               └─ capabilities
        ├─ PkgManager                 ├─ ProxyHostManager
        └─ ServiceManager             └─ DashboardManager
```

Domain targets are a future concern. The REST transport + escape hatch steps
are enough to start. Users build domain logic in Starlark helper functions,
same way they'd use `run()` before a native step type exists.

---

### 4. Docker Container Step

**Status:** No container management. Would require `run()` escape hatch today.

**The problem:** Docker containers are the most common deployment unit in
homelab/small-server setups. The pattern is dead consistent:

1. Create directories for volumes
2. Template config files
3. Deploy container (image, ports, volumes, env, restart policy, healthcheck)
4. Wait for healthy

Ansible's `community.docker.docker_container` module handles step 3. scampi
needs a native step for this because:
- It's the single highest-frequency operation in Docker-based setups
- The check/execute contract maps naturally (inspect container state vs desired)
- Healthcheck waiting is a first-class concern (deploy blocks that depend on a
  container being up need to know when it's ready)

**Design:**

```python
docker.container(
    name="prometheus",
    image="prom/prometheus:latest",
    ports=["9090:9090"],
    volumes=[
        "/opt/prometheus/config:/etc/prometheus",
        "/opt/prometheus/data:/prometheus",
    ],
    env={"TZ": "Europe/Copenhagen"},
    restart="always",
    healthcheck=docker.healthcheck(
        test="wget -qO- http://localhost:9090/-/healthy",
        interval="10s",
        timeout="5s",
        retries=3,
    ),
    networks=["monitoring"],
)
```

**Capability:** `ContainerManager` interface on the target. The SSH/local
target would implement it by shelling out to `docker` CLI — same approach as
`PkgManager` shelling out to `apt`/`dnf`.

**Check:** `docker inspect` the container. Compare image, ports, volumes, env,
restart policy. If anything differs, recreate.

**Execute:** `docker run` (or `docker compose up` for multi-container stacks).

**Open questions:**
- Compose support: a `docker.compose` step that manages a compose file? Or just
  the single-container primitive and compose is a `run()` + `template()`?
- Image pull policy: always pull? Only if not present? Configurable?
- Network creation: separate step (`docker.network`) or implicit?

---

### 5. User and Group Step

**Status:** No user/group management. Every host in the Ansible project runs an
`admin-user` role.

**The problem:** User creation is the most basic infrastructure primitive after
packages. Every host needs at least an admin user with SSH key, shell, groups,
and a password hash. This appears in literally every playbook.

**Design:**

```python
user(
    name="hal9000",
    shell="/bin/bash",
    password=secret("admin.password_hash"),
    groups=["sudo", "docker"],
    ssh_keys=["ssh-ed25519 AAAA..."],
)

group(
    name="appusers",
    gid=1100,
)
```

**Check:** `id <user>`, `getent group <group>`. Compare shell, groups, home
dir.

**Execute:** `useradd`/`usermod`, `groupadd`/`groupmod`. Password via
`chpasswd` or `usermod -p`. SSH keys via `authorized_keys` file management.

**Capability:** `UserManager` interface. Implemented by shelling out to POSIX
user management commands on SSH/local targets.

**Open questions:**
- Password handling: accept a pre-hashed value (like Ansible's
  `password_hash()` filter)? Or hash in scampi with a configurable algorithm?
- SSH key management: append to `authorized_keys` or own the whole file?
- Home directory creation: implicit with `useradd -m` or explicit?

---

### 6. Repository Step

**Status:** Planned in pkg-design.md but not implemented.

**The problem:** Before you can `pkg(packages=["docker-ce"])`, you need the
Docker apt repository configured with its GPG key. This is the most common
pre-requisite pattern in real deployments — MongoDB, PostgreSQL, Docker,
Grafana, InfluxDB all need custom repos.

The full sequence: download GPG key, dearmor it, drop it in
`/etc/apt/keyrings/`, add a `.list` or `.sources` file, update cache. Five
steps that always travel together.

Already designed in `docs/pkg-design.md`. Needs implementation.

---

### 7. Handlers / Post-Change Triggers

**Status:** No mechanism for "do X if Y changed."

**The problem:** The classic pattern: template a config file, restart the
service if (and only if) the config actually changed. In Ansible, handlers
accumulate notifications during a play and fire at the end. In scampi, steps
execute sequentially but there's no way to conditionally run a step based on
whether a previous step made changes.

**Design space:**

This is a subtle one. The declarative model says "the service should be running
with this config." If the config changed, the service needs a restart to pick
it up — that's a convergence gap that the engine should close, not something the
user should wire up manually.

Option A: **Explicit triggers on steps**

```python
template(src="./nginx.conf.tmpl", dest="/etc/nginx/nginx.conf", ...),
service(name="nginx", state="running", restart_on=["/etc/nginx/nginx.conf"]),
```

The service step knows: "if this file was changed in a preceding step, restart."
This is a dependency between steps — file-based, same as the `Pather` interface
already supports for ordering.

Option B: **Service step auto-detects config changes**

The service step declares config file paths. If any of them were modified by
preceding ops (the engine knows this from check/execute results), it triggers a
restart. No explicit wiring by the user.

Option C: **Post-change hooks on any step**

```python
template(
    src="./nginx.conf.tmpl",
    dest="/etc/nginx/nginx.conf",
    on_change=[run(apply="systemctl restart nginx", always=True)],
)
```

This is the most flexible but also the most "Ansible handler"-like, which is
the pattern we're trying to improve on.

**Direction:** Option A is the likely sweet spot. It's explicit, composable, and
doesn't require the engine to infer relationships. The `Pather` interface
already provides the dependency information — extending it to trigger restarts
is natural.

---

## Step Types: Future

These are lower priority than the structural gaps above, but they round out the
step library for common infrastructure patterns.

| Step | What it does | Ansible equivalent |
|------|-------------|-------------------|
| `repo` | Package repository management | `apt_repository`, `yum_repository` |
| `mount` | NFS/CIFS mount management | `mount` module |
| `cron` | Cron job management | `cron` module |
| `sysctl` | Kernel parameter management | `sysctl` module |
| `firewall` | Firewall rule management | `ufw`, `firewalld` modules |
| `apt_key` | GPG key management for repos | `apt_key` module |

Each follows the same pattern: implement `StepType`, add a config struct with
tags, add a Starlark builtin, register in the engine. The architecture supports
this — it's implementation work, not design work.

---

## What This Enables

With the gaps above filled, the Ansible homelab translates like this:

```python
# targets.star
load("secrets.star", "s")

npm_host = target.ssh(name="npm", host="10.10.2.30", user="hal9000",
                      key=s.ssh_key)
npm_api  = target.npm(name="npm_api", base_url="http://10.10.2.30:81/api",
                      auth=rest.bearer(token_endpoint="/tokens",
                                       identity=s.npm_email,
                                       secret=s.npm_password))

# deploy.star
deploy("npm-infra",
    targets=["npm"],
    steps=[
        pkg(packages=["ca-certificates", "curl", "gnupg"]),
        # ... docker repo setup, docker install ...
        docker.container(
            name="nginx-proxy-manager",
            image="jc21/nginx-proxy-manager:2",
            ports=["80:80", "443:443", "81:81"],
            volumes=["/opt/npm/data:/data", "/opt/npm/letsencrypt:/etc/letsencrypt"],
            healthcheck=docker.healthcheck(test="curl -f http://localhost:81"),
            restart="always",
        ),
    ],
)

deploy("npm-config",
    targets=["npm_api"],
    after=["npm-infra"],
    steps=[
        npm.proxy_host(domain="pihole.skrynet.dk", proxy_to="http://10.10.10.10:80",
                       ssl=True, auth="one_factor"),
        npm.proxy_host(domain="grafana.skrynet.dk", proxy_to="http://10.10.2.60:3000",
                       ssl=True, auth="two_factor"),
        # ... 25 more proxy hosts, generated from a list ...
    ],
)
```

No YAML. No Jinja. No vault-password prompt. No 22-level variable precedence.
No half-hour no-op runs. Check is fast, execute is minimal, re-runs are cheap.

---

## Sequencing

Not a promise of timeline — a dependency-ordered list of what to build and why.

**Phase 1: Foundation**
1. ~~Secrets~~ (done) — unblocks real-world configs
2. Deploy block dependencies (`after=`) — unblocks multi-phase workflows

**Phase 2: Targets**
3. REST target + auth — unblocks API-driven services
4. REST escape hatch steps — unblocks ad-hoc API operations

**Phase 3: Step Types**
5. Docker container — highest frequency gap
6. User/group — appears on every host
7. Repository — prereq for third-party packages

**Phase 4: Orchestration**
8. Handlers / post-change triggers — "restart if config changed"
9. Domain targets (NPM, Grafana, etc.) — convergence over REST APIs

Each phase builds on the previous. Phases 1 and 2 are structural — they change
how the engine works. Phases 3 and 4 are additive — new step types and target
types following established patterns.

---

## Non-Goals

Things this roadmap explicitly does not pursue:

- **Plugin system** — steps are built-in. If it's worth having, it's worth
  putting in the binary. This is a solo project, not an ecosystem.
- **Ansible compatibility layer** — no `ansible()` step type that wraps
  playbooks. The migration path is `run()` as a bridge, then native steps.
- **Dynamic inventory** — host lists are Starlark code. External data comes in
  via `env()`, `secret()`, or `load()`.  Starlark is a real language — you can
  read JSON files, build lists, compute values.
- **Agent mode** — scampi pushes from a control machine. No pull-based daemon.
- **Windows targets** — POSIX-first. Windows is a different world with
  different primitives. Maybe someday, but not on this roadmap.

---

## Rule of Thumb

> If you're reaching for `run()`, ask: "would a native step type make this
> declarative?" If yes, that's a signal to build the step type. If no, `run()`
> is doing its job.

The escape hatch is the adoption ramp. The roadmap is about paving that ramp
into a proper road.

---

## Odds and Ends

Things that aren't blocking anything but would be nice to get to.

- **Evaluate CLI parsers.** urfave/cli works fine but has rough edges — the help
  formatter for repeatable flags pushes description columns way right, and error
  output needs manual workarounds. Worth doing a comparison with cobra, kong, or
  ff at some point.
- **`scampi inspect` doesn't show template steps.** Templates should be
  inspectable like other step types.
- **`secrets reencrypt` command.** Decrypt all secrets with your key and
  re-encrypt them to an updated recipient list. Needed for key rotation and
  revoking access (e.g. team member leaves). Recipients file resolution:
  `--recipients` (inline CSV) or `--recipients-file` override, otherwise
  look for `<project_root>/.scampi/recipients` then
  `<project_root>/.recipients`. Each line is an age public key.
- **`check` across uncommitted changes.** When any step's check reports "would
  change", downstream steps that depend on its side effects will fail because
  the mutation hasn't happened yet. Example: a `dir` step "would create"
  `/etc/caddy`, but the `template` step that writes `/etc/caddy/Caddyfile`
  fails because the directory doesn't exist during check. Same problem with
  `run` steps across barriers. Instead of hard errors, the engine should report
  these as "skipped — depends on uncommitted changes." Requires the check phase
  to propagate "uncommitted" state through dependency edges.
- **Action-started feedback.** The CLI shows nothing between "plan finished" and
  the first action completing. Every action should announce itself when it starts
  executing, not just when it finishes — otherwise slow steps (apt installs,
  large file transfers) look like hangs.
- **Service reload/restart.** The service step only supports `running`/`stopped`.
  No way to reload or restart a service after config changes — currently requires
  a `run(always=True)` workaround. Add `Reload()`/`Restart()` to
  `ServiceManager` and support `reloaded`/`restarted` states, or fold it into
  the handlers design.
- **Scriptable configs.** Allow `scampi CONFIG apply` in addition to
  `scampi apply CONFIG` — config file as first positional arg. Combined with a
  `#!/usr/bin/env scampi` shebang and `+x`, configs become self-contained
  executables: `./site.star apply`, `./site.star check`, `./site.star plan`.
- **Error message consistency pass.** Go through all error messages codebase-wide
  and make them self-documenting: say what's wrong, show correct syntax using
  values the user already provided.
- ~~**`secrets()` validation ordering.**~~ Fixed: backend validated before path.
- ~~**`secrets()` usable as a value expression.**~~ Fixed: declaration builtins
  (`secrets`, `deploy`, `target.*`) return a poison pill value.
