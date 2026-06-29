// SPDX-License-Identifier: GPL-3.0-only

package spec

import "scampi.dev/scampi/internal/signal"

// ResourceKind identifies the type of a promised or deferred resource.
type ResourceKind uint8

const (
	ResourcePath ResourceKind = iota
	ResourceUser
	ResourceGroup
	ResourceRef
	ResourceLabel // arbitrary user-named resource (e.g. "realm:skrynet.lan")
)

// Resource is a typed key for a promised or deferred resource.
type Resource struct {
	Kind ResourceKind
	Name string
}

func PathResource(name string) Resource  { return Resource{Kind: ResourcePath, Name: name} }
func UserResource(name string) Resource  { return Resource{Kind: ResourceUser, Name: name} }
func GroupResource(name string) Resource { return Resource{Kind: ResourceGroup, Name: name} }
func LabelResource(name string) Resource {
	return Resource{Kind: ResourceLabel, Name: name}
}

// Promiser is an optional interface that steps can implement to declare
// resources they consume and produce. Used for automatic dependency
// inference and check-mode deferral.
type Promiser interface {
	Inputs() []Resource
	Promises() []Resource
}

// StaticInputProvider is implemented by TargetKinds that consume resources
// produced by other deploy blocks. The engine uses this to order plans
// cross-deploy: a deploy block whose target inputs a resource waits for
// whichever block promises it. Pure config inspection: no live connections,
// no probes.
type StaticInputProvider interface {
	StaticInputs(cfg any) []Resource
}

// StaticPromiseProvider is implemented by StepKinds that produce resources
// visible to other deploy blocks, consumed by a sibling block's target inputs.
// Pure config inspection. The op-level Promiser intra-step surface stays
// separate: those run after Plan(); this is pre-plan.
type StaticPromiseProvider interface {
	StaticPromises(cfg any) []Resource
}

// ResourceDeclarer is implemented by step Config structs that expose
// user-driven `promises = [...]` / `inputs = [...]` fields (e.g. posix.run,
// posix.service). The engine reads these alongside type-driven StaticPromises
// to build the cross-deploy resource graph: dc1's `samba-ad-dc` service can
// promise `realm:skrynet.lan`, and dc2's join step can input it, so the engine
// orders dc2 after dc1. Each declared name maps to a LabelResource; matching is
// exact-string. See #275.
type ResourceDeclarer interface {
	ResourceDeclarations() (promises, inputs []string)
}

// DriftDetail describes one field that differs between desired and current
// state, surfaced during Check.
type DriftDetail struct {
	Field     string
	Current   string
	Desired   string
	Verbosity signal.Verbosity // minimum verbosity to display (zero = always shown with drift)
}
