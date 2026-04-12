---
title: Testing
weight: 6
---

`scampi test` runs test files against mock targets. Test files are regular
scampi configs — same `module`, same `import`, same `std.deploy(...)` — with
test builtins for setting up initial state and declaring expected outcomes.

The key design: **the mock IS the assertion.** You declare `initial` state
(what the system looks like before the deploy) and `expect` state (what should
be true after), and the runner verifies automatically. There is no assertion
API to chain.

## Writing tests

A test file is any `*_test.scampi` file. It constructs a mock target with
optional initial and expected state, deploys steps against it, and lets the
runner verify the outcome:

```scampi {filename="webserver_test.scampi"}
module main

import "std"
import "std/test"
import "std/test/matchers"
import "std/posix"

let mock = test.target_in_memory(
  name    = "mock",
  initial = test.InitialState {
    packages = ["nginx"]
    services = {"nginx": posix.ServiceState.stopped}
  },
  expect = test.ExpectedState {
    files    = {"/etc/nginx/sites-enabled/example.com.conf": matchers.has_substring("server_name example.com")}
    services = {"nginx": matchers.has_svc_status(posix.ServiceState.running)}
    dirs     = {"/var/www/mysite": matchers.is_present()}
  },
)

std.deploy(name = "webserver", targets = [mock]) {
  // ... steps that configure nginx for example.com
}
```

The runner applies the deploy, then compares each slot in `expect` against the
mock's recorded state. Mismatches emit typed diagnostics.

## Running tests

```text
scampi test                        # *_test.scampi in current directory
scampi test ./...                  # recursive from current directory
scampi test path/to/test.scampi    # specific file
scampi test path/to/dir            # all *_test.scampi in that directory
scampi test path/to/dir/...        # recursive from that directory
```

All verifications run even if some fail — you see the full picture in one run.
Exit code 0 if all pass, 1 if any fail.

## Sections

{{< cards >}}
  {{< card link="targets" title="Targets" subtitle="In-memory and REST mock targets for testing" >}}
  {{< card link="assertions" title="Matchers" subtitle="Content, existence, and status matchers for expected state" >}}
{{< /cards >}}
