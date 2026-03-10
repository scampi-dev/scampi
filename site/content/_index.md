---
title: scampi
layout: hextra-home
---

{{< hextra/hero-badge link="https://codeberg.org/pskry/scampi" >}}
  <span>Open Source</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Declare the what.&nbsp;<br class="sm:hx-block hx-hidden" />scampi handles the how.
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-subtitle >}}
  IaC convergence, garlic buttery smooth 🍤
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mt-8 hx-mb-16" style="padding-top: 1.5rem; padding-bottom: 1.5rem;">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
</div>

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
{{< /hextra/feature-grid >}}
