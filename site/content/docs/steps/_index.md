---
title: Step Reference
weight: 4
---

Steps are the building blocks of scampi configs. Each step declares a piece of
desired state — scampi figures out what needs to change and makes it so.

Every step supports an optional `desc` field for a human-readable description.
This shows up in CLI output and is useful when you have many similar steps.

## Optional fields and drift

Step fields fall into three categories:

- **Required** — must be provided. The step won't plan without it.
- **Optional with default** — can be omitted; scampi uses the documented default
  (e.g. `state` defaults to `"present"`).
- **Optional, unmanaged** — can be omitted; scampi does not track the field at
  all. No opinion at creation, no drift detection afterwards.

This last category matters. When you omit an optional field like `shell` on a
`user` step, scampi doesn't secretly pick a value and then flip-flop on
subsequent runs. The OS picks whatever default it wants at creation time, and
scampi leaves it alone. If you want scampi to manage it, set it explicitly.

## Built-in steps

{{< cards >}}
  {{< card link="copy" title="copy" subtitle="Copy files with owner and permission management" >}}
  {{< card link="dir" title="dir" subtitle="Ensure a directory exists" >}}
  {{< card link="firewall" title="firewall" subtitle="Manage firewall rules via UFW or firewalld" >}}
  {{< card link="group" title="group" subtitle="Manage system groups" >}}
  {{< card link="pkg" title="pkg" subtitle="Manage system packages" >}}
  {{< card link="run" title="run" subtitle="Run arbitrary shell commands" >}}
  {{< card link="service" title="service" subtitle="Manage services (systemd, OpenRC, launchctl)" >}}
  {{< card link="sysctl" title="sysctl" subtitle="Manage kernel parameters" >}}
  {{< card link="symlink" title="symlink" subtitle="Create and manage symbolic links" >}}
  {{< card link="template" title="template" subtitle="Render templates with data substitution" >}}
  {{< card link="user" title="user" subtitle="Manage system user accounts" >}}
{{< /cards >}}
