# Changelog

## v0.1.0-alpha.7 — 2026-04-14

### Breaking Changes
- Migrate testkit test framework to scampi-lang (#153)
- lang: switch comment syntax to // and /* */ (#154)
- Rethink target module naming: posix mixes steps and targets (#168)

### Features
- scampi-lang: implement lexer (#133)
- scampi-lang: implement parser and AST (#134)
- scampi-lang: implement module resolver (#135)
- scampi-lang: implement type system and type checker (#136)
- scampi-lang: generate std library stubs from Go struct tags (#137)
- scampi-lang: opaque type declarations (`type Foo`) (#142)
- scampi-lang: compiler reads std stubs from embedded FS (#143)
- scampi-lang: linker stage for stub resolution (#144)
- scampi-lang: block invocation syntax for Deploy (#145)
- scampi-lang: stub generator produces correct multi-module output (#146)
- scampi-lang: eval resolves all module members from stubs (#147)
- Migrate testkit test framework to scampi-lang (#153)
- lang: param attributes (@name(args)) for declarative checker, linker, and LSP behavior (#159)
- lsp: goto-def on a struct-literal field jumps to the parameter declaration (#161)
- lang: UFCS — `x.f(args)` desugars to `f(x, args)` (#171)
- linker: resolve user module imports from scampi.mod (#176)
- std.ref() builtin for cross-step value references (#177)
- linker: implement remote module resolution (#180)
- lsp: reload modules when scampi.mod changes (#181)

### Enhancements
- LSP catalog metadata must derive from engine, not duplicate it (#131)
- scampi-lang: eval produces generic values, no engine types (#151)
- lang: switch comment syntax to // and /* */ (#154)
- Rethink target module naming: posix mixes steps and targets (#168)

### Bug Fixes
- nvim: commentstring empty for scampi files (#157)
- firewall step doesn't escalate, broken for non-root SSH targets (#169)
- fileops.VerifiedWrite silently mishandles multi-%s verify commands (#170)
- lang: funcs with declared return types are not required to return (#172)
- lang: duplicate let in same scope is silently allowed (#173)

### Other
- scampi-lang: resolve open spec questions before implementation (#132)
- scampi-lang: validate against .infra/ configs (#138)
- Kill the stub generator, make stubs the source of truth (#163)
- Delete implementation-detail tests that catch no real bugs (#164)

## v0.1.0-alpha.6 — 2026-04-02

### Breaking Changes
- Rename .star to .scampi file extension (#121)

### Features
- scampi.mod: module manifest format and parser (#105)
- scampi.sum: integrity checksums for module dependencies (#106)
- Module fetching: download modules from git repositories (#107)
- load() resolver: resolve module paths from scampi.mod (#108)
- scampi mod: CLI subcommands for module management (#109)
- scampi test: Starlark-native testing framework for modules (#110)
- Transitive dependency resolution (#111)
- Vanity import paths for scampi modules (#116)
- test.target.rest_mock: HTTP mock target for startest (#117)
- Starlark LSP for scampi configs (#118)

### Enhancements
- Rename .star to .scampi file extension (#121)
- gen api: add --prefix flag for path prefixing (#126)

### Bug Fixes
- secrets UX: first-use experience is broken (#115)
- StarlarkError missing source span for pre-builtin errors (#124)
- rest.resource cannot compose with generated API wrappers (#127)
- gen api: interpolate path parameters into Starlark expressions (#128)
- Deploy blocks stored in map — non-deterministic execution order (#129)

### Other
- Module structure conventions and documentation (#112)

## v0.1.0-alpha.5 — 2026-03-29

### Features
- rest.resource: declarative REST resource management with query/found/missing (#100)
- ref(): runtime value references between steps (#101)
- scampi gen: code generation subcommand (#104)

### Enhancements
- Diagnostic source spans point to call site, not offending token (#102)

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

