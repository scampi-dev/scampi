---
title: CLI
weight: 7
---

Scampi's CLI is organized into subcommands, each covering a different phase of
the convergence workflow.

## Subcommands

| Command             | Description                                            |
| ------------------- | ------------------------------------------------------ |
| [apply](apply)      | Converge system state to match the configuration       |
| [check](check)      | Inspect actual system state against the configuration  |
| [plan](plan)        | Show the execution plan without touching targets       |
| [inspect](inspect)  | Diff desired file content against current target state |
| [gen](gen)          | Generate Starlark modules from external schemas        |
| [mod](mod)          | Manage module dependencies                             |
| [test](test)        | Run Starlark test files                                |
| [index](step-index) | Browse built-in step documentation                     |
| [legend](legend)    | Print the CLI visual language reference card           |
| [secrets](secrets)  | Manage age-encrypted secrets                           |

## Global flags

These flags apply to all subcommands:

| Flag      | Description                                                  |
| --------- | ------------------------------------------------------------ |
| `-v`      | Increase verbosity (`-v` why, `-vv` how, `-vvv` everything)  |
| `--color` | Colorize output: `auto`, `always`, `never` (default: `auto`) |
| `--ascii` | Force ASCII output — disable Unicode glyphs                  |

## Exit codes

| Code | Meaning                                                     |
| ---- | ----------------------------------------------------------- |
| 0    | Success                                                     |
| 1    | User error — invalid config, failed plan, validation errors |
| 2    | Internal bug — panic or unexpected error                    |

## Output semantics

Scampi's CLI output is designed to be scannable. Colors are semantic, not
decorative, and verbosity controls how much detail you see.

### Colors

| Color   | Meaning                                        |
| ------- | ---------------------------------------------- |
| Yellow  | Mutation — something changed                   |
| Green   | Already correct — no change needed             |
| Red     | Failure                                        |
| Blue    | Deploy block boundaries — plan start/finish    |
| Cyan    | Action boundaries — step headers and op counts |
| Magenta | Plan structure — plan headers and rails        |
| Dim     | Detail — shown at higher verbosity levels      |

When you see yellow, something changed. When you see green, it was already right.
When you see red, something broke. You never need to read the text to understand
the outcome — the color tells you.

### Verbosity

| Flag   | Level | Shows                                                               |
| ------ | ----- | ------------------------------------------------------------------- |
| (none) | 0     | Outcomes only — what changed, what was already correct, what failed |
| `-v`   | 1     | *Why* — reasons for changes and skip decisions                      |
| `-vv`  | 2     | *How* — command output, diff details                                |
| `-vvv` | 3     | Everything — full execution trace                                   |

The default output is designed for the common case: you ran `apply` and want to
know what happened. Each verbosity level adds context without replacing the
previous level.

### Display options

| Flag      | Description                                                 |
| --------- | ----------------------------------------------------------- |
| `--color` | Force color output on or off (`--color=false` for no color) |
| `--ascii` | Use ASCII glyphs instead of Unicode symbols                 |

## Philosophy

Scampi's output avoids patterns common in other tools:

- **No progress bars** — operations are fast enough that progress indication adds
  noise rather than value.
- **No tree nesting** — output is a flat stream. Actions print in order. Failures
  are immediately visible.
- **Non-deterministic op ordering** — ops within an action run in parallel when
  their dependencies allow it. The order ops appear in output can vary between
  runs. `scampi plan` shows the dependency graph so you can see what runs
  concurrently.
