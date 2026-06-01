# scampi

[![CI](https://github.com/scampi-dev/scampi/actions/workflows/ci.yml/badge.svg)](https://github.com/scampi-dev/scampi/actions/workflows/ci.yml)
[![CodeQL](https://github.com/scampi-dev/scampi/actions/workflows/codeql.yml/badge.svg)](https://github.com/scampi-dev/scampi/actions/workflows/codeql.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/scampi-dev/scampi)](go.mod)
[![License: GPL v3](https://img.shields.io/badge/License-GPL_v3-blue.svg)](LICENSE)

A decentralized reconciler for bare-metal infrastructure.

scampi runs as a peer-to-peer mesh of agents that converge bare metal,
hypervisors, OS configuration, K8s day-0 bootstrap, and dumb network gear
toward a declared desired state. Think "lightweight K8s for bare metal":
the K8s reconciliation pattern applied to everything K8s doesn't reach.

No central control plane, no leader, no quorum nodes. Each peer runs the
same statically-linked binary and reconciles the resources it owns based
on placement labels and per-resource gossip leases.

## Design

The architectural spec lives in [`doc/design/2026-05-28-decentralized-reconciler.md`](doc/design/2026-05-28-decentralized-reconciler.md).

## License

GPL-3.0. See [LICENSE](LICENSE).
