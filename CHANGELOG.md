# Changelog

## v0.1.0-alpha.4 — 2026-03-25

### Features
- Step: mount (NFS/CIFS) (#17)
- REST target + auth (#2)
- REST escape hatch steps (#3)
- Docker container step (#4)
- container.instance: add env field (#84)
- container.instance: add mounts field for bind mounts (#85)
- container.instance: add healthcheck support (#87)
- container.instance: add args field for CLI arguments (#88)
- container.instance: add labels field (#89)
- revisit scampi inspect: show planned state before apply (#93)

### Enhancements
- Per-op timeouts instead of blanket action timeout (#58)
- Move SourceStore and DocFromConfig out of spec/ (#74)
- Remove unused idx field from action structs (#76)
- Deduplicate engine scheduling and Check/Apply command methods (#77)
- Deduplicate render summary formatting and fileops check paths (#78)
- Design debt: plan() complexity, registry allocation, sharedops boundaries (#79)
- Minor code quality sweep (#80)
- container.instance: replace raw port strings with domain type (#90)
- replace raw checksum strings with domain type (#91)
- firewall: replace raw port/proto strings with domain type (#92)
- lint: ban fmt.Errorf in user-facing error paths (#96)
- Built-in fuzzy finder for inspect --diff (#99)

### Bug Fixes
- Thread context through Starlark builtins instead of context.Background() (#72)
- Hardcoded Unicode bypasses glyphSet — breaks ASCII fallback (#73)
- reloadOp: eliminate mutable state between Check and Execute (#75)
- Recursive Check only inspects root path, not children (#81)

### Other
- Lazy target creation for plan and inspect list mode (#98)

## v0.1.0-alpha.3 — 2026-03-20

### Features
- Step: pkg_repo — manage package repository sources (#50)
- Implicit package cache management for pkg step (#54)
- get.scampi.dev install endpoint (#68)

### Enhancements
- Step: automatic daemon-reload on unit file changes (#53)
- Replace stringly-typed closed sets with proper enums (#59)
- Deduplicate target/local and target/ssh implementations (#61)
- Reduce boilerplate in engine package (#62)
- Normalize step implementation patterns (#63)
- Audit source.Source write methods against read-only boundary (#64)
- Minor code polish: helpers, complexity, readability (#65)
- Custom 404 page for scampi.dev (#70)

### Bug Fixes
- SSH target assumes GNU/Linux for escalated stat (#66)
- SSH test container not cleaned up after test run (#67)
- Verify temp file must preserve original filename (#69)

## v0.1.0-alpha.2 — 2026-03-18

### Features
- Auto-escalate to sudo on permission errors (#46)
- Source resolvers: unified file acquisition for all steps (#55)
- Source resolver: remote() (#56)

### Enhancements
- scampi index: wrap step descriptions instead of truncating (#36)
- Generalize promised resources beyond paths (#43)

### Bug Fixes
- Graceful cancellation on SIGINT instead of panic (#45)

## v0.1.0-alpha.1 — 2026-03-17

### Features
- Step: sysctl (#19)
- Step: firewall (#20)
- Verify field for copy and template steps (#35)
- Step: unarchive (#40)
- Automated site deployment on release (#48)
- User/group step (#5)
- Post-change hooks (#7)

### Enhancements
- Action-started feedback (#10)
- Service reload/restart (#11)
- Error message consistency pass (#16)
- Unify three copy-pasted cycle detection implementations (#32)
- scampi index should show default values for optional fields (#33)
- Inline content for copy step (symmetry with template) (#34)
- Add benchmark and fuzz coverage for all step types (#37)
- Proper action dependency system (#38)
- Deduplicate engine test fixtures (#39)

### Bug Fixes
- scampi inspect doesn't show template steps (#14)
- Fix goroutine leak in benchmark suite (#42)
- Check across uncommitted changes (#9)

