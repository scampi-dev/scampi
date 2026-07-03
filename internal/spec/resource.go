// SPDX-License-Identifier: GPL-3.0-only

package spec

// ResourceKind identifies the type of a provided or deferred resource.
type ResourceKind uint8

const (
	ResourcePath ResourceKind = iota
	ResourceUser
	ResourceGroup
	ResourceRef
	ResourceLabel // arbitrary user-named resource (e.g. "realm:skrynet.lan")
)

// Resource is a typed key for a provided or deferred resource.
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

// Requirer is an optional interface that steps can implement to declare
// resources they require. Used for automatic dependency inference and
// check-mode deferral.
type Requirer interface {
	Requires() []Resource
}

// Provider is an optional interface that steps can implement to declare
// resources they provide. Used for automatic dependency inference and
// check-mode deferral.
type Provider interface {
	Provides() []Resource
}

// StaticRequirer is implemented by TargetKinds that require resources
// provided by other deploy blocks. The engine uses this to order plans
// cross-deploy: a deploy block whose target requires a resource waits for
// whichever block provides it. Pure config inspection: no live connections,
// no probes.
type StaticRequirer interface {
	StaticRequires(cfg any) []Resource
}

// StaticProvider is implemented by StepKinds that provide resources
// visible to other deploy blocks, required by a sibling block's target
// requirements. Pure config inspection. The step-level Requirer/Provider
// surface stays separate: those run after Plan(); this is pre-plan.
type StaticProvider interface {
	StaticProvides(cfg any) []Resource
}

// ResourceDeclarer is implemented by step Config structs that expose
// user-driven `provides = [...]` / `requires = [...]` fields (e.g. posix.run,
// posix.service). The engine reads these alongside type-driven StaticProvides
// to build the cross-deploy resource graph: dc1's `samba-ad-dc` service can
// provide `realm:skrynet.lan`, and dc2's join step can require it, so the
// engine orders dc2 after dc1. Each declared name maps to a LabelResource;
// matching is exact-string. See #275.
type ResourceDeclarer interface {
	ResourceDeclarations() (provides, requires []string)
}

// DriftDetail describes one field that differs between desired and current
// state, surfaced during Check. Producers report data only; what to show at
// which verbosity is the renderer's call.
type DriftDetail struct {
	Field   string
	Current string
	Desired string
}
