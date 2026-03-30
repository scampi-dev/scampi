---
title: test
weight: 7
---

Run Starlark test files. See [Testing](/docs/modules/testing) for the full guide
on writing tests.

```text
scampi test [path]
```

| Argument          | Behavior                                   |
| ----------------- | ------------------------------------------ |
| (none)            | Run `*_test.star` in the current directory |
| `./...`           | Recursive from current directory           |
| `path/to/dir`     | All `*_test.star` in that directory        |
| `path/to/dir/...` | Recursive from that directory              |
| `path/file.star`  | Run a specific test file                   |

Hidden directories (starting with `.`) are skipped during recursive discovery.

## Output

Default output shows only failures. Use `-v` for verbose output showing every
assertion. All output respects `--color` and `--ascii` flags.

## Exit codes

| Code | Meaning                       |
| ---- | ----------------------------- |
| 0    | All assertions passed         |
| 1    | One or more assertions failed |
