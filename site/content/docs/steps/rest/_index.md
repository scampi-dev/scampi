---
title: rest
---

HTTP-based convergence for REST APIs. Used with
[REST targets]({{< relref "../../targets/rest" >}}) to configure applications
that expose an HTTP interface — reverse proxies, monitoring tools, DNS
providers, container orchestrators.

Requests support composable [check matchers]({{< relref "request#check-matchers" >}})
for idempotency — scampi queries the API before executing and skips the
mutation if the desired state already exists.

## Steps

{{< cards >}}
  {{< card link="request" title="request" subtitle="Make HTTP requests with optional idempotency checks" >}}
  {{< card link="resource" title="resource" subtitle="Declarative REST resource management with query/found/missing" >}}
{{< /cards >}}
