# Terminology

scampi's core types live in two worlds. The name prefix tells you which.

The pipeline runs from scampi source to executed ops:

```
lang (lex -> parse -> check -> eval)  -> eval.Result                     (evaluate)
linker.Link                           -> DeclaredConfig                  (link)
engine.Resolve                        -> Config (one per deploy×target)  (select)
StepKind.Plan                         -> Plan{ Deploy{ Step{ Op } } }    (plan, then execute)
```

The `lang` pipeline (lexer, parser, checker, evaluator) is isolated on purpose:
it never imports `engine` or `target`. Its output, `eval.Result`, is the
boundary the linker consumes to produce the first typed world, `DeclaredConfig`.

## The two worlds

| Lifecycle slot           | Config           | Deploy           | Step           | Op     | Target           |
| ------------------------ | ---------------- | ---------------- | -------------- | ------ | ---------------- |
| per-kind Go type         | (none)           | (none)           | `StepKind`     | (none) | `TargetKind`     |
| declared (linker output) | `DeclaredConfig` | `DeclaredDeploy` | `DeclaredStep` | (none) | `DeclaredTarget` |
| execution (engine runs)  | `Config`         | `Deploy`         | `Step`         | `Op`   | `target.Target`  |

- **Declarative** types (`Declared*`) are the user's intent, exactly as the
  language linker emits them. Nothing is resolved or planned yet.
- **Execution** types are bare nouns. They are what the engine runs, and they
  appear everywhere, so they keep the short canonical names.
- **`Config`** is the hinge: `engine.Resolve` splits one `DeclaredConfig` into
  one `Config` per (deploy, target). A `Config`'s steps are still
  `DeclaredStep`; planning is what turns each into an executable `Step`.

Every bare name above lives in package `spec`. `target.Target` is the one
exception: the live connection (the mutable surface), kept in package `target`
on purpose, separate from the descriptive `spec` types.

## Per-kind types

One implementation per kind, registry-backed:

- **`StepKind`**: the Go type for a step kind. `Plan(DeclaredStep) -> Step`.
- **`TargetKind`**: the Go type for a target kind. `Create(...) -> target.Target`.

`Kind` (not `Type`) because each is the thing you implement once per kind, and
`Type` already means "a node in the type system" over in `lang/check`.

## Enum naming

Default to `*Kind` for our own discriminator enums (`ResourceKind`, `CauseKind`,
`ScopeKind`, `SymbolKind`, `ErrKind`, `PkgSourceKind`, `SourceRefKind`).
Use `*Type` only when it mirrors a hard external term.

## Avoid

`Impl`, `Handler`, `Spec`, and bare `Instance` / `Type` / `Resolved` as
disambiguation suffixes (naming something `XType` only because `X` was taken).
