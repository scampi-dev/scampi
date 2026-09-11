// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

// deployNode is one resolved-config plan plus the resources it
// provides and requires, used to build the cross-deploy DAG.
type deployNode struct {
	idx      int // index into the Config slice
	res      spec.Config
	provides []spec.Resource
	requires []spec.Resource
	deps     []*deployNode // upstream nodes this one waits for
}

// deployGraph is a level-by-level plan schedule. Each level is a slice
// of nodes that have no remaining dependencies; nodes within a level
// can run concurrently. Level boundaries enforce ordering: every node
// in level N has finished before any node in level N+1 starts.
type deployGraph struct {
	levels [][]*deployNode
}

// buildDeployGraph computes provider/requirer relationships across
// resolved deploy blocks via the static Provider/Requirer surface and
// topo-sorts them into execution levels.
//
// External requirements (a node requires a resource that no node in
// this run provides) are treated as already-satisfied - those nodes
// become roots. If the resource genuinely doesn't exist at runtime,
// downstream errors surface that cleanly.
func buildDeployGraph(resolved []spec.Config) (*deployGraph, error) {
	nodes := make([]*deployNode, len(resolved))
	for i, r := range resolved {
		nodes[i] = &deployNode{
			idx:      i,
			res:      r,
			provides: collectProvides(r),
			requires: collectRequires(r),
		}
	}

	// Build producer index. Multiple producers for the same resource
	// is ambiguous and must be flagged.
	producer := make(map[spec.Resource][]*deployNode)
	for _, n := range nodes {
		for _, p := range n.provides {
			producer[p] = append(producer[p], n)
		}
	}
	for r, prods := range producer {
		if len(prods) > 1 {
			return nil, MultipleProvidersError{
				Resource: r,
				Deploys:  deployNamesOf(prods),
			}
		}
	}

	// Wire deps. Requirements without a producer in this run are
	// external - no edge added.
	for _, n := range nodes {
		for _, in := range n.requires {
			prods := producer[in]
			if len(prods) == 0 {
				continue
			}
			n.deps = append(n.deps, prods[0])
		}
	}

	if cycle := findCycle(nodes); cycle != nil {
		return nil, DeployCycleError{Deploys: deployNamesOf(cycle)}
	}

	return &deployGraph{levels: kahnLevels(nodes)}, nil
}

func collectProvides(r spec.Config) []spec.Resource {
	var out []spec.Resource
	for _, step := range r.Steps {
		// Type-driven: a step kind auto-provides resources from its config.
		if p, ok := step.Type.(spec.StepKindResources); ok {
			out = append(out, p.ProvidesFor(step.Config)...)
		}
		// Config-driven user labels (e.g. posix.service { provides = ["..."] }).
		if d, ok := step.Config.(spec.ConfigResources); ok {
			provides, _ := d.Resources()
			for _, p := range provides {
				out = append(out, spec.LabelResource(p))
			}
		}
	}
	return out
}

func collectRequires(r spec.Config) []spec.Resource {
	var out []spec.Resource
	// Target-driven: a target kind requires resources from its config.
	if p, ok := r.Target.Type.(spec.TargetKindResources); ok {
		out = append(out, p.RequiresFor(r.Target.Config)...)
	}
	// Config-driven user labels on steps (e.g. posix.run { requires = ["..."] }).
	for _, step := range r.Steps {
		d, ok := step.Config.(spec.ConfigResources)
		if !ok {
			continue
		}
		_, requires := d.Resources()
		for _, in := range requires {
			out = append(out, spec.LabelResource(in))
		}
	}
	return out
}

// kahnLevels does a Kahn-style topo sort returning levels of nodes
// with the same depth (max distance from any root). Level 0 is the
// roots; level N+1 contains nodes whose deepest dep was in level N.
// This preserves maximum within-level parallelism.
func kahnLevels(nodes []*deployNode) [][]*deployNode {
	depth := make(map[*deployNode]int, len(nodes))
	maxDepth := 0
	for _, n := range nodes {
		d := depthOf(n, depth)
		if d > maxDepth {
			maxDepth = d
		}
	}
	levels := make([][]*deployNode, maxDepth+1)
	for _, n := range nodes {
		levels[depth[n]] = append(levels[depth[n]], n)
	}
	return levels
}

func depthOf(n *deployNode, memo map[*deployNode]int) int {
	if d, ok := memo[n]; ok {
		return d
	}
	d := 0
	for _, dep := range n.deps {
		if c := depthOf(dep, memo) + 1; c > d {
			d = c
		}
	}
	memo[n] = d
	return d
}

// findCycle detects cycles via DFS. Returns the cycle (in order) if
// found, nil otherwise.
func findCycle(nodes []*deployNode) []*deployNode {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // fully explored
	)
	color := make(map[*deployNode]int, len(nodes))
	var stack []*deployNode

	var visit func(n *deployNode) []*deployNode
	visit = func(n *deployNode) []*deployNode {
		color[n] = gray
		stack = append(stack, n)
		for _, dep := range n.deps {
			switch color[dep] {
			case white:
				if c := visit(dep); c != nil {
					return c
				}
			case gray:
				// Found a back edge - slice the stack from dep to here.
				start := 0
				for i, s := range stack {
					if s == dep {
						start = i
						break
					}
				}
				cycle := append([]*deployNode(nil), stack[start:]...)
				return cycle
			}
		}
		color[n] = black
		stack = stack[:len(stack)-1]
		return nil
	}

	for _, n := range nodes {
		if color[n] != white {
			continue
		}
		if c := visit(n); c != nil {
			return c
		}
	}
	return nil
}

func deployNamesOf(ns []*deployNode) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.res.DeployName + "/" + n.res.TargetName
	}
	return out
}

// MultipleProvidersError fires when two or more deploy blocks declare
// they provide the same resource. Ambiguous ordering would let either
// run first, so this is fatal at link time.
type MultipleProvidersError struct {
	Resource spec.Resource
	Deploys  []string
}

func (e MultipleProvidersError) Error() string {
	return fmt.Sprintf(
		"resource %s:%s is provided by multiple deploy blocks: %s",
		resourceKindName(e.Resource.Kind), e.Resource.Name,
		strings.Join(e.Deploys, ", "),
	)
}

func (e MultipleProvidersError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID: CodeMultipleProviders,
			Text: `resource {{.Resource}} is provided by multiple deploy blocks: ` +
				`{{range $i, $d := .Deploys}}{{if $i}}, {{end}}{{$d}}{{end}}`,
			Hint: "ensure only one deploy block creates this resource",
			Data: e,
		},
	}
}

// DeployCycleError fires when deploy blocks form a circular dependency
// through their resource graph (A requires a resource provided by B,
// and B requires one provided by A).
type DeployCycleError struct {
	Deploys []string
}

func (e DeployCycleError) Error() string {
	return fmt.Sprintf("deploy block cycle: %s", strings.Join(e.Deploys, " -> "))
}

func (e DeployCycleError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID: CodeDeployCycle,
			Text: `circular dependency between deploy blocks: ` +
				`{{range $i, $d := .Deploys}}{{if $i}} -> {{end}}{{$d}}{{end}}`,
			Hint: "break the cycle by splitting or merging deploy blocks",
			Data: e,
		},
	}
}

// displayResource formats a resource for user-facing output as "[kind]name",
// e.g. "[path]/etc/app", "[label]shared:ready". The bracketed kind stays
// visible but is visually distinct from the name, so a value that contains a
// colon (a label like "realm:skrynet.lan") is never mistaken for "kind:name".
func displayResource(r spec.Resource) string {
	return "[" + resourceKindName(r.Kind) + "]" + r.Name
}

func resourceKindName(k spec.ResourceKind) string {
	switch k {
	case spec.ResourcePath:
		return "path"
	case spec.ResourceUser:
		return "user"
	case spec.ResourceGroup:
		return "group"
	case spec.ResourceRef:
		return "ref"
	case spec.ResourceLabel:
		return "label"
	default:
		return "unknown"
	}
}
