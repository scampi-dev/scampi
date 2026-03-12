---
title: Step Reference
weight: 4
---

Steps are the building blocks of scampi configs. Each step declares a piece of
desired state — scampi figures out what needs to change and makes it so.

Every step supports an optional `desc` field for a human-readable description.
This shows up in CLI output and is useful when you have many similar steps.

## Built-in steps

{{< cards >}}
  {{< card link="copy" title="copy" subtitle="Copy files with owner and permission management" >}}
  {{< card link="dir" title="dir" subtitle="Ensure a directory exists" >}}
  {{< card link="pkg" title="pkg" subtitle="Manage system packages" >}}
  {{< card link="service" title="service" subtitle="Manage services (systemd, OpenRC, launchctl)" >}}
  {{< card link="symlink" title="symlink" subtitle="Create and manage symbolic links" >}}
  {{< card link="template" title="template" subtitle="Render templates with data substitution" >}}
  {{< card link="run" title="run" subtitle="Run arbitrary shell commands" >}}
{{< /cards >}}
