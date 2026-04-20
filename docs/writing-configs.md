# Writing scampi configs

scampi is a declarative system convergence engine. You describe desired
state; it makes reality match.

## Reference material (read in this order)

1. `site/content/docs/reference.md` — cheatsheet: syntax, steps, enums
2. `site/content/docs/language/_index.md` — full language guide
3. `site/content/docs/steps/` — per-step detailed docs with examples
4. `site/content/docs/targets/` — target types (local, SSH, REST)

## Key patterns

- 2-space indent, no tabs
- Step blocks use braces + `=` assignments
- Bare steps in deploy blocks are desired state
- `let`-bound steps are values for `on_change`
- Enums are always qualified: `posix.ServiceState.running`
- Secrets never inline — always through resolvers
