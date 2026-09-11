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

// Resource graph declarations
// -----------------------------------------------------------------------------
//
// Five interfaces across (implementer, phase, direction). They look alike but
// do not overlap - the name says which cell:
//
//	StepProvides         planned Step   post-plan  Provides()
//	StepRequires         planned Step   post-plan  Requires()
//	StepKindResources    StepKind       pre-plan   ProvidesFor(cfg)
//	TargetKindResources  TargetKind     pre-plan   RequiresFor(cfg)
//	ConfigResources      Config struct  pre-plan   Resources() -> user strings
//
// All five are declarations: they report facts, they never do work. Each is
// optional, and not implementing one is itself the declaration - so an
// implementation never returns nil just to satisfy the shape.

// StepProvides is an optional interface that planned steps implement to
// declare resources they provide. Used for automatic dependency inference
// and check-mode deferral. Implement it only if the step actually provides
// something - absence is the declaration that it does not.
type StepProvides interface {
	Provides() []Resource
}

// StepRequires is an optional interface that planned steps implement to
// declare resources they require. Same rules as StepProvides: implement it
// only if the step actually requires something.
type StepRequires interface {
	Requires() []Resource
}

// TargetKindResources is implemented by TargetKinds that require resources
// provided by other deploy blocks. The engine uses this to order plans
// cross-deploy: a deploy block whose target requires a resource waits for
// whichever block provides it. Pure config inspection: no live connections,
// no probes.
type TargetKindResources interface {
	RequiresFor(cfg any) []Resource
}

// StepKindResources is implemented by StepKinds that provide resources
// visible to other deploy blocks, required by a sibling block's target
// requirements. Pure config inspection. Distinct from StepProvides: that one
// runs after Plan() on a planned step, this one is pre-plan on the kind.
type StepKindResources interface {
	ProvidesFor(cfg any) []Resource
}

// ConfigResources is implemented by step Config structs that expose
// user-driven `provides = [...]` / `requires = [...]` fields (e.g. posix.run,
// posix.service). The engine reads these alongside type-driven StepKindResources
// to build the cross-deploy resource graph: dc1's `samba-ad-dc` service can
// provide `realm:skrynet.lan`, and dc2's join step can require it, so the
// engine orders dc2 after dc1. Each declared name maps to a LabelResource;
// matching is exact-string. See #275.
type ConfigResources interface {
	Resources() (provides, requires []string)
}

// DriftDetail describes one field that differs between desired and current
// state, surfaced during Check. Producers report data only; what to show at
// which verbosity is the renderer's call.
type DriftDetail struct {
	Field   string
	Current string
	Desired string
}
