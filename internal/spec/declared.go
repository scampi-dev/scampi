// SPDX-License-Identifier: GPL-3.0-only

package spec

import (
	"context"

	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/target"
)

// DeclaredConfig is the whole linker output: every named target and every
// deploy block the user wrote, in declaration order. Nothing is selected or
// planned yet. engine.Resolve turns it into one Config per (deploy, target)
// pair.
type DeclaredConfig struct {
	Path    string
	Targets map[string]DeclaredTarget // named targets
	Deploy  []DeclaredDeploy          // deploy blocks (declaration order)
}

// DeclaredDeploy is one `deploy { }` block as written: the target names it
// fans out to, its ordered steps, and its notify-only hook steps.
type DeclaredDeploy struct {
	Name    string                    // block name (key from deploy map)
	Targets []string                  // references target names
	Steps   []DeclaredStep            // ordered steps
	Hooks   map[string][]DeclaredStep // hook ID → steps (execute only when notified)
	Source  SourceSpan                // source location
}

// Config is one resolved run unit: a single deploy block paired with a single
// target, the unit the engine plans and executes. It is the hinge between the
// declared and execution worlds, so its Steps are still DeclaredStep; planning
// is what turns them into executable Steps.
type Config struct {
	Path       string
	DeployName string                    // which deploy block
	TargetName string                    // which target
	Target     DeclaredTarget            // resolved target
	Steps      []DeclaredStep            // steps from the deploy block
	Hooks      map[string][]DeclaredStep // hook ID → steps (execute only when notified)
}

// DeclaredTarget is a target as written: its kind, decoded config, and source
// span. No connection is open yet; TargetKind.Create turns it into a live
// target.Target at execution time.
type DeclaredTarget struct {
	Type   TargetKind
	Config any
	Source SourceSpan
	Fields map[string]FieldSpan
}

// DeclaredStep is a step as written: its kind, decoded config, and source span.
// StepKind.Plan turns it into an executable Step.
type DeclaredStep struct {
	ID       StepID // unique identifier, assigned during scampi eval
	Desc     string // optional human description
	Type     StepKind
	Config   any
	OnChange []string // hook IDs to notify when this step changes
	Source   SourceSpan
	Fields   map[string]FieldSpan
}

// TargetKind is the Go type representing a target kind (one per kind). It
// decodes target configuration and creates live target connections.
type TargetKind interface {
	Kind() string
	NewConfig() any
	Create(ctx context.Context, src source.Source, tgt DeclaredTarget) (target.Target, error)
}

// StepKind is the Go type representing a step kind (one per kind). It decodes
// step configuration and plans execution.
type StepKind interface {
	Kind() string
	// NewConfig MUST return a pointer to a freshly allocated config struct.
	// Returning a value will cause undefined behavior.
	NewConfig() any
	Plan(step DeclaredStep) (Step, error)
}

// ResolveOptions controls deploy block and target selection.
type ResolveOptions struct {
	// DeployNames filters to specific deploy blocks (empty = all)
	DeployNames []string
	// TargetNames filters to specific targets (empty = all in deploy block)
	TargetNames []string
}

// DeployByName returns the deploy block with the given name, or false.
func (c DeclaredConfig) DeployByName(name string) (DeclaredDeploy, bool) {
	for _, b := range c.Deploy {
		if b.Name == name {
			return b, true
		}
	}
	return DeclaredDeploy{}, false
}
