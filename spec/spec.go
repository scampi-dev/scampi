// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=CheckResult
package spec

import (
	"context"
	"strconv"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

// ResourceKind identifies the type of a promised or deferred resource.
type ResourceKind uint8

const (
	ResourcePath ResourceKind = iota
	ResourceUser
	ResourceGroup
	ResourceRef
	ResourceContainer
	ResourceLXC   // PVE LXC container, Name = vmid as string
	ResourceLabel // arbitrary user-named resource (e.g. "realm:skrynet.lan")
)

// Resource is a typed key for a promised or deferred resource.
type Resource struct {
	Kind ResourceKind
	Name string
}

func PathResource(name string) Resource      { return Resource{Kind: ResourcePath, Name: name} }
func UserResource(name string) Resource      { return Resource{Kind: ResourceUser, Name: name} }
func GroupResource(name string) Resource     { return Resource{Kind: ResourceGroup, Name: name} }
func ContainerResource(name string) Resource { return Resource{Kind: ResourceContainer, Name: name} }
func LXCResource(vmid int) Resource {
	return Resource{Kind: ResourceLXC, Name: strconv.Itoa(vmid)}
}
func LabelResource(name string) Resource {
	return Resource{Kind: ResourceLabel, Name: name}
}

type (
	Config struct {
		Path    string
		Targets map[string]TargetInstance // named targets
		Deploy  []DeployBlock             // deploy blocks (declaration order)
	}

	DeployBlock struct {
		Name    string                    // block name (key from deploy map)
		Targets []string                  // references target names
		Steps   []StepInstance            // ordered steps
		Hooks   map[string][]StepInstance // hook ID → steps (execute only when notified)
		Source  SourceSpan                // source location
	}

	ResolvedConfig struct {
		Path       string
		DeployName string                    // which deploy block
		TargetName string                    // which target
		Target     TargetInstance            // resolved target
		Steps      []StepInstance            // steps from the deploy block
		Hooks      map[string][]StepInstance // hook ID → steps (execute only when notified)
	}

	// TargetType is the Go type representing a target kind (one per kind).
	// It decodes target configuration and creates live target connections.
	TargetType interface {
		Kind() string
		NewConfig() any
		Create(ctx context.Context, src source.Source, tgt TargetInstance) (target.Target, error)
	}
	// StaticInputProvider is implemented by TargetTypes that consume
	// resources produced by other deploy blocks. The engine uses this
	// to order plans cross-deploy: a deploy block whose target inputs
	// `lxc:1000` waits for whichever block promises it. Pure config
	// inspection — no live connections, no probes.
	StaticInputProvider interface {
		StaticInputs(cfg any) []Resource
	}
	TargetInstance struct {
		Type   TargetType
		Config any
		Source SourceSpan
		Fields map[string]FieldSpan
	}

	UnitInstance struct {
		ID   UnitID
		Desc string
	}
	StepInstance struct {
		ID       StepID // unique identifier, assigned during scampi eval
		Desc     string // optional human description
		Type     StepType
		Config   any
		OnChange []string // hook IDs to notify when this step changes
		Source   SourceSpan
		Fields   map[string]FieldSpan
	}
	// StaticPromiseProvider is implemented by StepTypes that produce
	// resources visible to other deploy blocks (e.g. pve.lxc creating
	// an LXC consumed by a sibling block's lxc_target). Pure config
	// inspection. The op-level Promiser intra-action surface stays
	// separate — those run after Plan(), this is pre-plan.
	StaticPromiseProvider interface {
		StaticPromises(cfg any) []Resource
	}
	// ResourceDeclarer is implemented by step Config structs that
	// expose user-driven `promises = [...]` / `inputs = [...]` fields
	// (e.g. posix.run, posix.service). The engine reads these
	// alongside type-driven StaticPromises to build the cross-deploy
	// resource graph: dc1's `samba-ad-dc` service can promise
	// `realm:skrynet.lan`, and dc2's join step can input it, so the
	// engine orders dc2 after dc1. Each declared name maps to a
	// LabelResource — matching is exact-string. See #275.
	ResourceDeclarer interface {
		ResourceDeclarations() (promises, inputs []string)
	}
	// StepType is the Go type representing a step kind (one per kind).
	// It decodes step configuration and plans execution.
	StepType interface {
		Kind() string
		// NewConfig MUST return a pointer to a freshly allocated config struct.
		// Returning a value will cause undefined behavior.
		NewConfig() any
		Plan(step StepInstance) (Action, error)
	}
	FieldSpan struct {
		Field SourceSpan
		Value SourceSpan
	}
	SourceSpan struct {
		Filename  string
		StartLine int
		EndLine   int
		StartCol  int
		EndCol    int
	}

	Plan struct {
		Unit Unit
	}
	UnitID string
	Unit   struct {
		ID      UnitID
		Desc    string
		Target  target.Target
		Actions []Action
	}
	// Action groups the ops for a single step into a parallel execution DAG.
	Action interface {
		Desc() string // optional human description
		Kind() string
		Ops() []Op
	}
	// Promiser is an optional interface that actions can implement to declare
	// resources they consume and produce. Used for automatic dependency
	// inference and check-mode deferral.
	Promiser interface {
		Inputs() []Resource
		Promises() []Resource
	}
	// SourceReader is an optional interface that actions can implement to
	// declare source-side files they read. The engine pre-caches these so
	// the renderer can display source context in error messages.
	SourceReader interface {
		SourcePaths() []string
	}
	// Op is the smallest idempotent unit of work. Ops receive both
	// Source (host/config state) and Target (system being converged).
	Op interface {
		Action() Action
		Check(ctx context.Context, src source.Source, tgt target.Target) (CheckResult, []DriftDetail, error)
		Execute(ctx context.Context, src source.Source, tgt target.Target) (Result, error)
		DependsOn() []Op
		RequiredCapabilities() capability.Capability
	}
	DriftDetail struct {
		Field     string
		Current   string
		Desired   string
		Verbosity signal.Verbosity // minimum verbosity to display (zero = always shown with drift)
	}

	// OpTimeout is an optional interface that ops can implement to declare
	// a per-op timeout. Ops that don't implement it get the default.
	OpTimeout interface {
		Timeout() time.Duration
	}
	// Diffable is an optional interface that ops producing file content
	// can implement to support `scampi inspect --diff`.
	Diffable interface {
		DesiredContent(ctx context.Context, src source.Source) ([]byte, error)
		CurrentContent(ctx context.Context, src source.Source, tgt target.Target) ([]byte, error)
		DestPath() string
	}
	// InspectField is a single label/value pair exposed by OpInspector.
	InspectField struct {
		Label string
		Value string
	}
	// OpInspector is an optional interface that ops can implement to expose
	// their resolved state for `scampi inspect`.
	OpInspector interface {
		Inspect() []InspectField
	}
	OpDescriber interface {
		OpDescription() OpDescription
	}

	// Deduplicatable is an optional interface that step configs can implement
	// to enable dedup when the same logical step appears multiple times in a
	// steps list (e.g. returned from multiple helper functions). Steps with
	// the same Kind + DedupKey are collapsed to one; refs to dropped IDs are
	// remapped to the survivor.
	Deduplicatable interface {
		DedupKey() string
	}
	// OutputProvider is an optional interface that ops can implement to
	// expose their settled state after execution. The engine captures this
	// for ref() resolution in downstream steps.
	OutputProvider interface {
		Output() any
	}
	OpDescription interface {
		PlanTemplate() PlanTemplate
	}

	PlanTemplate struct {
		ID   string
		Text string
		Data any
	}

	Result struct {
		Changed bool
	}

	CheckResult uint8

	// ResolveOptions controls deploy block and target selection.
	ResolveOptions struct {
		// DeployNames filters to specific deploy blocks (empty = all)
		DeployNames []string
		// TargetNames filters to specific targets (empty = all in deploy block)
		TargetNames []string
	}
)

func (t PlanTemplate) TemplateID() string   { return t.ID }
func (t PlanTemplate) TemplateText() string { return t.Text }
func (t PlanTemplate) TemplateData() any    { return t.Data }

const (
	CheckUnknown CheckResult = iota
	CheckSatisfied
	CheckUnsatisfied
)

// DeployByName returns the deploy block with the given name, or false.
func (c Config) DeployByName(name string) (DeployBlock, bool) {
	for _, b := range c.Deploy {
		if b.Name == name {
			return b, true
		}
	}
	return DeployBlock{}, false
}
