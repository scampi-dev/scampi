---
title: About
---

<style>
@font-face {
  font-family: 'NerdSymbols';
  src: url('/fonts/nerd-symbols-subset.woff2') format('woff2');
  font-display: swap;
  unicode-range: U+F012C, U+F03EB, U+F040A, U+F0026, U+F0156, U+F069C, U+F02D5, U+F0476;
}
.mission {
  font-size: 1.25rem;
  font-weight: 500;
  line-height: 1.6;
  border-left: 4px solid var(--primary-color, #3b82f6);
  padding-left: 1rem;
  margin: 2rem 0;
}
.term {
  border-radius: 8px;
  padding: 1rem 1.25rem;
  font-family: 'NerdSymbols', 'SF Mono', 'Cascadia Code', 'Fira Code', monospace;
  font-size: 0.82rem;
  line-height: 1.35;
  overflow-x: auto;
  margin: 0.75rem 0;
  white-space: pre;
  background: #fbf1c7;
  color: #3c3836;
}
.term .cmd { color: #3c3836;                    }
.term .g   { color: #79740e;                    }
.term .gd  { color: #79740e; opacity: 0.65;     }
.term .y   { color: #b57614;                    }
.term .r   { color: #9d0006;                    }
.term .b   { color: #076678;                    }
.term .bb  { color: #076678; font-weight: bold; }
.term .c   { color: #427b58; opacity: 0.8;      }
.term .d   { color: #928374;                    }
.term .m   { color: #8f3f71; opacity: 0.8;      }
.term .mb  { color: #8f3f71; font-weight: bold; }
:root.dark .term {
  background: #282828;
  color: #ebdbb2;
}
:root.dark .term .cmd { color: #ebdbb2;                    }
:root.dark .term .g   { color: #b8bb26;                    }
:root.dark .term .gd  { color: #b8bb26; opacity: 0.65;     }
:root.dark .term .y   { color: #fabd2f;                    }
:root.dark .term .r   { color: #fb4934;                    }
:root.dark .term .b   { color: #83a598;                    }
:root.dark .term .bb  { color: #83a598; font-weight: bold; }
:root.dark .term .c   { color: #8ec07c; opacity: 0.8;      }
:root.dark .term .d   { color: #928374;                    }
:root.dark .term .m   { color: #d3869b; opacity: 0.8;      }
:root.dark .term .mb  { color: #d3869b; font-weight: bold; }
.term-label {
  font-size: 0.78rem;
  font-weight: 600;
  color: #888;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-top: 1.5rem;
  margin-bottom: 0.25rem;
}
</style>

## Mission

<div class="mission">

Scampi exists so you can describe your infrastructure once, in a real
language, and trust that reality will match — today, tomorrow, and after
the next reboot.

</div>

## The elevator pitch

You have servers. They need packages installed, config files in the right
place, services running, firewall rules set, containers launched. Today you
do it with a mix of shell scripts, YAML playbooks, and crossing your
fingers.

**Scampi replaces all of that with one config file.**

```python
target.ssh(name="web", host="app.example.com", user="deploy")

deploy(
    name = "webserver",
    targets = ["web"],
    steps = [
        pkg(packages=["nginx", "certbot"], state="present"),
        copy(
            src = local("./nginx.conf"),
            dest = "/etc/nginx/nginx.conf",
            perm = "0644", owner = "root", group = "root",
        ),
        service(name="nginx", state="started", enabled=True),
        firewall(ports=["80/tcp", "443/tcp"], state="open"),
    ],
)
```

That's it. That's a production webserver config. Run `scampi apply` and
the machine converges to that state. Run it again — nothing happens,
because it's already correct.

### What makes it different

**It's a real language, not YAML.** Scampi configs are
[Starlark](https://github.com/google/starlark-go) — a deterministic,
Python-like language designed by Google for configuration. You get
variables, functions, loops, and conditionals without the "is this a
string or a boolean" surprises of YAML. No templating nightmares. No
looping over included files just to repeat a task.

**Batteries included.** Packages, files, templates, services, containers,
firewall rules, symlinks, archives, mounts, REST APIs — it's all built in.
No plugin ecosystems to navigate, no modules to install, version-pin,
and pray haven't been abandoned. One binary, everything works.

**No agents.** No daemons, no control plane, no coordination service.
Scampi runs locally or connects over SSH, does its thing, and leaves.
No residual runtime, no background process, no scheduled pull.

**Actually idempotent.** Every operation checks before it acts. Scampi
doesn't "run a script and hope" — it inspects current state, computes a
diff, and only touches what's wrong. The check/apply cycle is the core
abstraction, not an afterthought bolted onto shell commands.

**Immediate convergence.** When `scampi apply` finishes, the target is in
the declared state. Not eventually, not after a reconciliation loop, not
when a controller gets around to it. Right now, confirmed, done.

**Targets aren't just machines.** A target can be your local machine, a
remote host over SSH, or a REST API. REST targets are first-class —
authentication, TLS, and connection management are handled by the target,
not cobbled together per-request in your config. The same check/apply
convergence model works whether you're writing files to a server or
ensuring a resource exists in an API.

**Errors that teach.** When something's wrong in your config, scampi
doesn't dump a stack trace and wish you luck. It tells you what's wrong,
shows you what the correct syntax looks like using your own values, and
guides you to a working config one error at a time.

**Testable without deploying.** Most infra tools' idea of testing is
"apply it and see what breaks." Scampi has a built-in test framework with
mock targets that run in milliseconds — no containers, no cloud accounts,
no YAML scaffolding. Write a `*_test.scampi` file, assert on the outcome,
done. REST API modules get mock targets too, with full request recording.

**Editor support that actually works.** `scampls` is an LSP server that
runs the real evaluation pipeline — not a syntax checker, the actual
Starlark evaluator with the full step registry. Unknown fields, missing
required params, type errors, invalid enums — all caught as you type, with
precise source spans and "did you mean?" hints. Your editor becomes the
first line of defense, not `apply`.

### The workflow

```mermaid
flowchart LR
    A["<b>DECLARE</b><br/>write config"] --> B["<b>PLAN</b><br/>what will happen"]
    B --> C["<b>CHECK</b><br/>what needs to change"]
    C --> D["<b>APPLY</b><br/>converge"]
    D -->|"run again"| C

    style A stroke:#c678dd,stroke-width:2px
    style B stroke:#61afef,stroke-width:2px
    style C stroke:#e5c07b,stroke-width:2px
    style D stroke:#73c991,stroke-width:2px
```

### See it in action

Here's the full cycle on a simple config that creates directories, copies a
file, and sets up a symlink.

```python {filename="demo.scampi"}
target.local(name="my-machine")

deploy(
    name = "demo",
    targets = ["my-machine"],
    steps = [
        dir(path="/tmp/scampi-demo", perm="0755"),
        dir(path="/tmp/scampi-demo/releases/v1", perm="0755"),
        copy(
            src = inline("welcome to scampi\n"),
            dest = "/tmp/scampi-demo/releases/v1/README",
            perm = "0644", owner = "root", group = "root",
        ),
        symlink(
            target = "/tmp/scampi-demo/releases/v1",
            link = "/tmp/scampi-demo/current",
        ),
    ],
)
```

<div class="term-label">1. Plan — see the execution structure</div>
<div class="term"><span class="cmd">$ scampi plan -v demo.scampi</span>
<span class="gd">[engine] started</span>
<span class="b">[plan] planning</span> <span class="bb">demo</span> <span class="b">— 4 steps planned</span>
<span class="m">┌─┬</span> <span class="mb">execution plan</span>
<span class="m">│</span> <span class="c">┏━┯ [0] dir</span>
<span class="m">│</span> <span class="d">┇ └─builtin.dir</span>
<span class="m">│</span> <span class="d">┇   └─builtin.ensure-mode</span>
<span class="m">│</span> <span class="c">■</span>
<span class="m">│</span> <span class="c">┏━┯ [1] dir</span>      <span class="d">← [0]</span>
<span class="m">│</span> <span class="d">┇ └─builtin.dir</span>
<span class="m">│</span> <span class="d">┇   └─builtin.ensure-mode</span>
<span class="m">│</span> <span class="c">■</span>
<span class="m">│</span> <span class="c">┏━┯ [2] copy</span>     <span class="d">← [0, 1]   ╮ ⏸</span>
<span class="m">│</span> <span class="d">┇ └─builtin.copy-file       │</span>
<span class="m">│</span> <span class="d">┇   ├─builtin.ensure-owner  │</span>
<span class="m">│</span> <span class="d">┇   └─builtin.ensure-mode   │</span>
<span class="m">│</span> <span class="c">■</span>                           <span class="d">│</span>
<span class="m">│</span> <span class="c">┏━┯ [3] symlink</span>  <span class="d">← [1, 0]   │</span>
<span class="m">│</span> <span class="d">┇ └─builtin.symlink         │</span>
<span class="m">│</span> <span class="c">■</span>                           <span class="d">╯</span>
<span class="m">└─■</span>
</div>

<p style="font-size: 0.78rem; color: #7f848e; line-height: 1.5; margin: 0.5rem 0 1rem;">
[1] depends on [0] (subdirectory). [2] depends on both.
[3] only needs [1], not [2] — so [2] and [3] run in parallel
(the ╮⏸╯ group). Inside each step, ops form a DAG: the copy writes
the file first, then sets permissions and ownership concurrently.
</p>

<div class="term-label">2. Check — inspect what would change (yellow = needs change)</div>
<div class="term"><span class="cmd">$ scampi check -v demo.scampi</span>
<span class="c">[dir:0] started...</span>
<span class="d">[dir:0] 󰏫 builtin.dir — needs change</span>
<span class="d">         state: (missing) → directory</span>
<span class="d">[dir:0] 󰏫 builtin.ensure-mode — needs change</span>
<span class="d">         perm: (missing) → -rwxr-xr-x</span>
<span class="y">[dir:0] 󰏫 2/2 ops would change</span>
<span class="c">[dir:1] started...</span>
<span class="d">[dir:1] 󰏫 builtin.dir — needs change</span>
<span class="d">         state: (missing) → directory</span>
<span class="y">[dir:1] 󰏫 2/2 ops would change</span>
<span class="c">[copy:2] started...</span>
<span class="d">[copy:2] 󰏫 builtin.copy-file — needs change</span>
<span class="d">[copy:2] 󰏫 builtin.ensure-mode — needs change</span>
<span class="d">         perm: (missing) → -rw-r--r--</span>
<span class="d">[copy:2] 󰏫 builtin.ensure-owner — needs change</span>
<span class="d">         owner:group: (missing) → root:root</span>
<span class="y">[copy:2] 󰏫 3/3 ops would change</span>
<span class="c">[symlink:3] started...</span>
<span class="d">[symlink:3] 󰏫 builtin.symlink — needs change</span>
<span class="y">[symlink:3] 󰏫 1/1 ops would change</span>
<span class="y">[engine] finished (4 would change, 0 failed)</span>
</div>

<div class="term-label">3. Apply — converge</div>
<div class="term"><span class="cmd">$ scampi apply -v demo.scampi</span>
<span class="c">[dir:0] started...</span>
<span class="d">[dir:0] 󰐊 'builtin.dir' changed</span>
<span class="y">[dir:0] 󰏫 1/2 ops changed</span>
<span class="c">[dir:1] started...</span>
<span class="d">[dir:1] 󰐊 'builtin.dir' changed</span>
<span class="y">[dir:1] 󰏫 1/2 ops changed</span>
<span class="c">[symlink:3] started...</span>
<span class="c">[copy:2] started...</span>
<span class="d">[symlink:3] 󰐊 'builtin.symlink' changed</span>
<span class="y">[symlink:3] 󰏫 1/1 ops changed</span>
<span class="d">[copy:2] 󰐊 'builtin.copy-file' changed</span>
<span class="d">[copy:2] 󰐊 'builtin.ensure-owner' changed</span>
<span class="y">[copy:2] 󰏫 2/3 ops changed</span>
<span class="y">[engine] finished (4 changed, 0 failed)</span>
</div>

<div class="term-label">4. Check again — everything green</div>
<div class="term"><span class="cmd">$ scampi check -v demo.scampi</span>
<span class="g">[dir:0] 󰄬 up-to-date</span>
<span class="g">[dir:1] 󰄬 up-to-date</span>
<span class="g">[copy:2] 󰄬 up-to-date</span>
<span class="g">[symlink:3] 󰄬 up-to-date</span>
<span class="g">[engine] finished (0 would change, 0 failed)</span>
</div>

That last block is the whole point. Run it tomorrow, run it after someone
manually pokes at the server, run it after a reboot — same result.

### Reading the output

Colors are semantic, not decorative. Run `scampi legend` for the full
reference — here's the short version:

<div class="term"><span class="cmd">$ scampi legend</span>
<span class="d">STATE</span>
  <span class="y">󰏫</span>  change    system state was modified
  <span class="g">󰄬</span>  ok        already correct, no change needed
  <span class="d">󰐊</span>  exec      operation executed
<span class="d">COLORS</span>
  <span class="y">yellow</span>    mutation, system state changed
  <span class="g">green</span>     correct, no change needed
  <span class="r">red</span>       failure
  <span class="b">blue</span>      engine and plan boundaries
  <span class="m">magenta</span>   plan structure
  <span class="c">cyan</span>      action context
  <span class="d">dim</span>       detail (higher verbosity)
</div>

## Want the deep cut?

Read the [Philosophy]({{< relref "docs/philosophy" >}}) for the design
principles — why convergence over imperative, why Starlark over YAML,
why testability and developer ergonomics aren't nice-to-haves.

Check out [Testing]({{< relref "docs/testing" >}}) to see how mock
targets and assertions work, or set up
[the LSP]({{< relref "docs/lsp" >}}) in your editor.

Or just [get started]({{< relref "docs/getting-started" >}}) and see for
yourself.
