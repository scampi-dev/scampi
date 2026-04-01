---
title: scampi
layout: hextra-home
---

{{< hextra/hero-badge link="https://codeberg.org/scampi-dev/scampi" >}}
  <span>Open Source</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<style>
.hero-row { display: flex; align-items: center; justify-content: center; position: relative; }
.hero-mascot { height: 360px; position: absolute; right: -360px; top: 0%; transform: translateY(-0%); }
@media (max-width: 768px) { .hero-mascot { display: none; } }
.home-code pre { overflow-x: auto; max-width: 100%; }
.home-code { overflow: hidden; max-width: 100%; }
</style>

<div class="hero-row hx-mt-6 hx-mb-6">
<div>

{{< hextra/hero-headline >}}
  Declare the what.&nbsp;<br class="sm:hx-block hx-hidden" />scampi handles the how.
{{< /hextra/hero-headline >}}

<div class="hx-mt-4">
{{< hextra/hero-subtitle >}}
  IaC convergence, garlic buttery smooth
{{< /hextra/hero-subtitle >}}
</div>

</div>
<img src="/scampi.png" alt="scampi mascot" class="hero-mascot">
</div>

<div class="hx-mt-8 hx-mb-16" style="padding-top: 1.5rem; padding-bottom: 1.5rem; display: flex; gap: 1rem; flex-wrap: wrap; align-items: center;">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
<a href="/get/" style="display: inline-flex; align-items: center; gap: 0.35rem; padding: 0.5rem 1.25rem; border-radius: 0.375rem; font-weight: 500; font-size: 0.95rem; border: 1px solid var(--border-color, #d1d5db); text-decoration: none; color: inherit;">Install <code style="font-size: 0.8rem; opacity: 0.7;">curl get.scampi.dev | sh</code></a>
</div>

<div class="home-code">

```python
target.local(name="my-machine")

deploy(
    name = "hello",
    targets = ["my-machine"],
    steps = [
        dir(path="/tmp/scampi-demo", perm="0755"),
        dir(path="/tmp/scampi-demo/v1", perm="0755"),
        symlink(
            target = "/tmp/scampi-demo/v1",
            link = "/tmp/scampi-demo/current",
        ),
    ],
)
```

</div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Declarative"
    style="pointer-events: none"
    subtitle="Describe the desired state of your systems in Starlark. Scampi figures out what needs to change."
  >}}
  {{< hextra/feature-card
    title="Idempotent"
    style="pointer-events: none"
    subtitle="Every operation checks before it acts. Run it once or a hundred times — same result."
  >}}
  {{< hextra/feature-card
    title="Agentless"
    style="pointer-events: none"
    subtitle="No daemons, no agents, no control plane. Just SSH and the binary."
  >}}
  {{< hextra/feature-card
    title="Starlark"
    style="pointer-events: none"
    subtitle="Python-like configuration language. Deterministic, hermetic, no surprises."
  >}}
  {{< hextra/feature-card
    title="Batteries Included"
    style="pointer-events: none"
    subtitle="Built-in steps for packages, files, templates, services, and more. No plugins to install."
  >}}
  {{< hextra/feature-card
    title="Fail-Fast"
    style="pointer-events: none"
    subtitle="Errors are caught early with clear messages that guide you to the fix. No silent failures."
  >}}
  {{< hextra/feature-card
    title="Testable"
    style="pointer-events: none"
    subtitle="Built-in test framework with mock targets. Test your infra code in milliseconds, no containers needed."
  >}}
  {{< hextra/feature-card
    title="Editor-First"
    style="pointer-events: none"
    subtitle="LSP server with real-time diagnostics, completion, and hover docs. Full eval pipeline, not just syntax."
  >}}
{{< /hextra/feature-grid >}}
