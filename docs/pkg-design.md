# Package Management Design

## Step Separation

Package-related operations split into distinct steps, each with a single
declarative intent and a tight check/execute contract.

### `pkg` — named packages from the system index

What we have today. "This package name should be present/absent." Backend is a
template struct (`fmt.Sprintf` with `%s` for package names). Check queries the
package database (`dpkg -s`, `rpm -q`, `brew list`, etc.).

The template-based `pkgmgr.Backend` struct works here because every package
manager follows the same command shape: `tool [flags] <names>`. No need for an
interface — unlike svcmgr where launchctl broke the template pattern.

If a future backend can't be expressed as a template (Nix flakes, for example),
that's when `pkgmgr.Backend` becomes an interface. Not before.

### `repo` — package sources

"This package source should be configured." Separate state, separate lifecycle.
A repo must exist before packages can be installed from it — natural DAG
dependency between steps.

Command shapes vary wildly across distros:
- `add-apt-repository ppa:user/repo`
- `dnf copr enable user/repo`
- `brew tap user/repo`
- Dropping a `.repo` file into `/etc/yum.repos.d/` (which is a file + cache refresh)

Some repo setups decompose into primitives we already have (copy a file, run a
command), but the declarative intent "this repo should be present" deserves its
own step kind with its own idempotency check.

### `pkg_file` — install from artifact (deb, rpm, tarball, URL)

"This specific artifact should be installed." Different model from `pkg`:
- Check: is this exact version/file installed? (may need to extract metadata
  from the artifact and compare against installed state)
- Install: `dpkg -i /path/to.deb`, `rpm -i /path/to.rpm`, etc.
- The `%s`-is-a-package-name assumption breaks — now it's a file path and the
  check logic changes shape.

Could be a mode within `pkg` (e.g. `src:` field vs `name:` field) but
internally needs distinct action logic regardless.

### AUR

AUR has a documented standard process — no reason to depend on third-party
helpers (yay, paru, etc.). The canonical workflow is:

1. `git clone https://aur.archlinux.org/<pkg>.git` into a build directory
2. `makepkg -si --noconfirm` inside the clone

Check: `pacman -Q <pkg>` still works (AUR packages land in the pacman DB).
This is a `pkg_file`-shaped operation (build from source, install the result)
rather than a `pkg`-shaped one (query an index). It could be its own step kind
(`aur`) or a mode of `pkg_file` with `src: "aur:<name>"`.

Must NOT run as root — `makepkg` refuses. The step needs to handle building as
a regular user and installing with escalation.

### snap / flatpak

These are separate package managers with their own install/check commands.
They map cleanly to `pkg`-style backends with different templates:
- snap: `snap list <pkg>` / `snap install <pkg>`
- flatpak: `flatpak info <pkg>` / `flatpak install -y <pkg>`

Each would be a backend (or step kind) rather than bolted onto `pkg`.

## Anti-pattern: Ansible's kitchen sink

Ansible's `apt` module handles packages, debs, dpkg options, cache updates,
autoremove, full-upgrade, and repo management in one module. Every other package
module copies this, resulting in dozens of flags that only make sense in certain
combinations. Error messages have to guess which mode you were in.

The step-per-intent model avoids this: each step composes through the DAG
instead of through flag combinations.
