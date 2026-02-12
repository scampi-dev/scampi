//go:generate stringer -type=CheckResult
package spec

import (
	"context"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/source"
	"godoit.dev/doit/target"
)

type (
	Config struct {
		Path    string
		Targets map[string]TargetInstance // named targets
		Deploy  map[string]DeployBlock    // deploy blocks
		Sources SourceStore
	}

	DeployBlock struct {
		Name    string         // block name (key from deploy map)
		Targets []string       // references target names
		Steps   []StepInstance // ordered steps
		Source  SourceSpan     // source location
	}

	ResolvedConfig struct {
		Path       string
		DeployName string         // which deploy block
		TargetName string         // which target
		Target     TargetInstance // resolved target
		Steps      []StepInstance // steps from the deploy block
		Sources    SourceStore
	}

	// TargetType is the Go handler for a CUE target kind (one per kind).
	TargetType interface {
		Kind() string
		NewConfig() any
		Create(ctx context.Context, src source.Source, tgt TargetInstance) (target.Target, error)
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
		Desc   string // optional human description
		Type   StepType
		Config any
		Source SourceSpan
		Fields map[string]FieldSpan
	}
	// StepType is the Go handler for a CUE step kind (one per kind).
	StepType interface {
		Kind() string
		// NewConfig MUST return a pointer to a freshly allocated config struct.
		// Returning a value will cause undefined behavior.
		NewConfig() any
		Plan(idx int, step StepInstance) (Action, error)
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
	// Pather is an optional interface that actions can implement to declare
	// their input/output paths for automatic dependency inference.
	Pather interface {
		// InputPaths returns paths this action reads from (source or target)
		InputPaths() []string
		// OutputPaths returns paths this action writes to (target only)
		OutputPaths() []string
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
		Field   string
		Current string
		Desired string
	}

	// Inspectable is an optional interface that ops producing file content
	// can implement to support `doit inspect`.
	Inspectable interface {
		DesiredContent(ctx context.Context, src source.Source) ([]byte, error)
		CurrentContent(ctx context.Context, src source.Source, tgt target.Target) ([]byte, error)
		DestPath() string
	}
	OpDescriber interface {
		OpDescription() OpDescription
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
		// InventoryPath is an explicit inventory file path
		InventoryPath string
		// EnvName loads inventory/<name>.cue and vars/<name>.cue
		EnvName string
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
