# CLI Semantics

`scampi`'s CLI output is designed for **parallel, unordered execution**. Colors and verbosity are not decoration — they are a **semantic contract**.

---

## Color Semantics

| Color   | Meaning                 | When                                              |
| ------- | ----------------------- | ------------------------------------------------- |
| Yellow  | Change / Mutation       | Something modified system state                   |
| Green   | Correctness / Stability | System was already correct; no change needed      |
| Red     | Failure                 | An error occurred; execution could not proceed    |
| Blue    | Deploy block boundaries | Plan start/finish, deploy block lifecycle         |
| Cyan    | Action boundaries       | Action headers, kind labels, op counts            |
| Magenta | Plan structure          | Plan headers, plan rails                          |
| Dim     | Detail / Noise          | Ops, checks, execution details (higher verbosity) |

---

## Verbosity Levels

Verbosity controls **how much explanation** you receive — never *what happened*.

| Level     | Shows                   | Use case                                              |
| --------- | ----------------------- | ----------------------------------------------------- |
| (default) | Outcomes only           | Changed actions, final summary                        |
| `-v`      | *Why* changes happened  | Unsatisfied checks, action headers, unchanged actions |
| `-vv`     | *How* changes happened  | Execution details, plan lifecycle                     |
| `-vvv`    | Full operational detail | Satisfied checks, all ops, everything                 |

Increasing verbosity **never removes information** — it only adds context.

---

## Design Philosophy

- Color answers **what happened**
- Verbosity answers **how much detail you want**
- Formatting never implies ordering or hierarchy
- Output stays readable under concurrency

When color is enabled, **all output is colored**. Uncolored text would imply neutrality that does not exist.

> Color conveys meaning. Verbosity adds explanation. The CLI never lies about execution.

---

## Why Not Ansible/Terraform Style Output?

Traditional tools assume:
- Linear execution
- Human-paced output
- Visual grouping implies execution order

`scampi` assumes the opposite:
- Actions and ops run in parallel
- Ordering is not stable or meaningful
- Grouping or indentation would lie about causality

As a result, `scampi` intentionally avoids:
- Animated or buffered output
- Progress bars or spinners
- Tree-style nesting that implies sequencing
- Color used as decoration

Instead, the CLI reports **facts**: what changed, what was correct, what failed, what context you're looking at.

> If the system is unordered and parallel, the output must be honest about that reality.
