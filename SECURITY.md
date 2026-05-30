# Security Policy

scampi reconciles desired state against real bare-metal infrastructure
with root privileges. It writes files, manages directories, and is
designed to grow into executing remote commands and managing secrets.
Vulnerability reports are taken seriously.

## Reporting a vulnerability

**Don't file public issues for security bugs.**

Primary channel: **security@scampi.dev**

For sensitive reports, encrypt to the project's PGP key:

```
Key ID:      2115C4F6AEEC665E
Fingerprint: 0BEE 3764 98D0 6547 7E16  DA71 2115 C4F6 AEEC 665E
```

Fetch the public key:

```
gpg --keyserver hkps://keys.openpgp.org --recv-keys 0BEE376498D065477E16DA712115C4F6AEEC665E
```

If email doesn't work for you, reach me however you can -- fediverse
DM, IRC, carrier pigeon, the bottom of a bottle washed up on a
Bornholm beach. I'd rather hear about a vulnerability weirdly than
not hear about it at all. Just don't open a public issue.

Please include:

- A description of the issue and your assessment of impact
- Steps to reproduce or a proof-of-concept
- The version of scampi you're running (`scampi --version`)

I'll acknowledge receipt within 3 business days and aim to provide a
status update within 7. For confirmed vulnerabilities, expect
coordinated disclosure with a 90-day timeline; severe issues may
warrant a shorter window. Reporters who request credit will be named
in release notes.

## In scope

- The `scampi` engine -- reconcile loop, snapshot handling, orphan
  eviction (`internal/engine`)
- HCL parser and resolver -- expression evaluation, ref extraction,
  ref folding against the Resolved store (`internal/hcl`)
- Source loading -- directory walk, file read, snapshot revision
  computation (`internal/source/file`)
- Target abstraction -- POSIX filesystem operations, mode and
  ownership handling (`internal/target`, `internal/target/posix`)
- Kind implementations -- `file`, `dir`, and any future Kinds that
  touch reality
- Action log -- persistence integrity, append-only invariants
  (`internal/diag.ActionLogSink`)
- The install pipeline (once one exists): release artifacts, signed
  binaries, `SHA256SUMS`

## Out of scope

- Third-party Go dependencies -- please file upstream first; only
  forward to scampi if a dependency vuln has scampi-specific impact
  not addressed upstream
- Example configs in `doc/`
- Issues that require an attacker to already have root on the target
  machine, or to already control the source directory contents
- Best-effort hardening suggestions ("you should also check X") --
  useful and welcome as regular issues, but not vulnerability reports

## Project status

scampi is pre-1.0 and solo-developed. There is no formal coordinated
disclosure infrastructure -- no CVE numbering authority, no PSIRT, no
bug bounty. I'll respond promptly and act in good faith. For a tool
that root-execs on user machines, that's the floor; the project will
graduate to a more formal process as adoption grows.
