// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// deployNode is one resolved-config plan plus the resources it
// produces and consumes, used to build the cross-deploy DAG.
type deployNode struct {
	idx      int // index into the ResolvedConfig slice
	res      spec.ResolvedConfig
	promises []spec.Resource
	inputs   []spec.Resource
	deps     []*deployNode // upstream nodes this one waits for
}

// deployGraph is a level-by-level plan schedule. Each level is a slice
// of nodes that have no remaining dependencies; nodes within a level
// can run concurrently. Level boundaries enforce ordering: every node
// in level N has finished before any node in level N+1 starts.
type deployGraph struct {
	levels [][]*deployNode
}

// buildDeployGraph computes producer/consumer relationships across
// resolved deploy blocks via the static Promiser/Inputs surface and
// topo-sorts them into execution levels.
//
// External inputs (a node consumes a resource that no node in this
// run produces) are treated as already-satisfied — those nodes
// become roots. If the resource genuinely doesn't exist at runtime,
// downstream errors (e.g. LxcUnreachableError on pve.lxc_target)
// surface that cleanly.
func buildDeployGraph(resolved []spec.ResolvedConfig) (*deployGraph, error) {
	nodes := make([]*deployNode, len(resolved))
	for i, r := range resolved {
		nodes[i] = &deployNode{
			idx:      i,
			res:      r,
			promises: collectPromises(r),
			inputs:   collectInputs(r),
		}
	}

	// Build producer index. Multiple producers for the same resource
	// is ambiguous and must be flagged.
	producer := make(map[spec.Resource][]*deployNode)
	for _, n := range nodes {
		for _, p := range n.promises {
			producer[p] = append(producer[p], n)
		}
	}
	for r, prods := range producer {
		if len(prods) > 1 {
			return nil, MultipleProducersError{
				Resource: r,
				Deploys:  deployNamesOf(prods),
			}
		}
	}

	// Wire deps. Inputs without a producer in this run are external —
	// no edge added.
	for _, n := range nodes {
		for _, in := range n.inputs {
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

func collectPromises(r spec.ResolvedConfig) []spec.Resource {
	var out []spec.Resource
	for _, step := range r.Steps {
		// Type-driven (e.g. pve.lxc auto-promises `lxc:<vmid>`).
		if p, ok := step.Type.(spec.StaticPromiseProvider); ok {
			out = append(out, p.StaticPromises(step.Config)...)
		}
		// Config-driven user labels (e.g. posix.service { promises = ["..."] }).
		if d, ok := step.Config.(spec.ResourceDeclarer); ok {
			promises, _ := d.ResourceDeclarations()
			for _, p := range promises {
				out = append(out, spec.LabelResource(p))
			}
		}
	}
	return out
}

func collectInputs(r spec.ResolvedConfig) []spec.Resource {
	var out []spec.Resource
	// Target-driven (e.g. pve.lxc_target consumes `lxc:<vmid>`).
	if p, ok := r.Target.Type.(spec.StaticInputProvider); ok {
		out = append(out, p.StaticInputs(r.Target.Config)...)
	}
	// Config-driven user labels on steps (e.g. posix.run { inputs = ["..."] }).
	for _, step := range r.Steps {
		d, ok := step.Config.(spec.ResourceDeclarer)
		if !ok {
			continue
		}
		_, inputs := d.ResourceDeclarations()
		for _, in := range inputs {
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
				// Found a back edge — slice the stack from dep to here.
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

// MultipleProducersError fires when two or more deploy blocks declare
// they produce the same resource. Ambiguous ordering would let either
// run first, so this is fatal at link time.
type MultipleProducersError struct {
	diagnostic.FatalError
	Resource spec.Resource
	Deploys  []string
}

func (e MultipleProducersError) Error() string {
	return fmt.Sprintf(
		"resource %s:%s has multiple producers: %s",
		resourceKindName(e.Resource.Kind), e.Resource.Name,
		strings.Join(e.Deploys, ", "),
	)
}

func (e MultipleProducersError) EventTemplate() event.Template {
	return event.Template{
		ID: CodeMultipleProducers,
		Text: `resource {{.Resource}} has multiple producers: ` +
			`{{range $i, $d := .Deploys}}{{if $i}}, {{end}}{{$d}}{{end}}`,
		Hint: "ensure only one deploy block creates this resource",
		Data: e,
	}
}

// DeployCycleError fires when deploy blocks form a circular dependency
// through their resource graph (A consumes a resource produced by B,
// and B consumes one produced by A).
type DeployCycleError struct {
	diagnostic.FatalError
	Deploys []string
}

func (e DeployCycleError) Error() string {
	return fmt.Sprintf("deploy block cycle: %s", strings.Join(e.Deploys, " -> "))
}

func (e DeployCycleError) EventTemplate() event.Template {
	return event.Template{
		ID: CodeDeployCycle,
		Text: `circular dependency between deploy blocks: ` +
			`{{range $i, $d := .Deploys}}{{if $i}} -> {{end}}{{$d}}{{end}}`,
		Hint: "break the cycle by splitting or merging deploy blocks",
		Data: e,
	}
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
	case spec.ResourceContainer:
		return "container"
	case spec.ResourceLXC:
		return "lxc"
	case spec.ResourceLabel:
		return "label"
	default:
		return "unknown"
	}
}
