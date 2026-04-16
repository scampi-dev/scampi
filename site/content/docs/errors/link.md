---
title: "link.*"
weight: 4
---

Link errors happen after type checking, when the linker connects the validated
config to the engine's runtime. The scampi code is valid — but it references
something the engine doesn't know how to execute.

## link.Unresolved

A step, target, or type declaration in the config has no matching handler
registered in the engine.

{{< err >}}
module main
import "custom"
custom.deploy { name = "web" } // ← no step type registered for custom.deploy
{{< /err >}}

This typically means:
- A standard library step name was misspelled
- A user module declares a `decl` that isn't backed by a Go step type
- The scampi binary was built without the step type you're trying to use

If you're using a custom module, make sure its step types are registered in
the engine registry.
