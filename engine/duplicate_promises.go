// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// detectDuplicatePromises rejects plans where two actions promise the
// same Resource. Two actions promising the same resource is always a
// user error: either two writers will race (e.g. two pve.lxc with the
// same VMID), or one masks the other (e.g. two posix.user "alice").
//
// The first action to promise a given resource is the "winner"; every
// later action promising the same resource produces a DuplicateResourceError
// pointing back at the original.
func detectDuplicatePromises(
	em diagnostic.Emitter,
	actions []spec.Action,
	actionSteps []int,
	steps []spec.StepInstance,
) error {
	winners := map[spec.Resource]int{} // resource → action index of first promiser
	var causes []error

	for i, act := range actions {
		p, ok := act.(spec.Promiser)
		if !ok {
			continue
		}
		for _, r := range p.Promises() {
			prevIdx, dup := winners[r]
			if !dup {
				winners[r] = i
				continue
			}

			cur := steps[actionSteps[i]]
			prev := steps[actionSteps[prevIdx]]
			err := DuplicateResourceError{
				Resource:     r,
				KindLabel:    resourceKindLabel(r.Kind),
				HintText:     resourceKindHint(r.Kind),
				StepKind:     cur.Type.Kind(),
				StepDesc:     cur.Desc,
				Source:       cur.Source,
				OtherKind:    prev.Type.Kind(),
				OtherDesc:    prev.Desc,
				OtherSource:  prev.Source,
				OtherLocText: formatSpan(prev.Source),
			}
			causes = append(causes, err)
			emitPlanDiagnostic(em, actionSteps[i], cur.Type.Kind(), cur.Desc, err)
		}
	}

	if len(causes) > 0 {
		return AbortError{Causes: causes}
	}
	return nil
}

// DuplicateResourceError fires when two actions promise the same Resource.
type DuplicateResourceError struct {
	diagnostic.FatalError
	Resource     spec.Resource
	KindLabel    string // human label for Resource.Kind (e.g. "container", "path")
	HintText     string // pre-computed hint (depends on Resource.Kind)
	StepKind     string
	StepDesc     string
	Source       spec.SourceSpan
	OtherKind    string
	OtherDesc    string
	OtherSource  spec.SourceSpan
	OtherLocText string // pre-formatted location of the original promiser
}

func (e DuplicateResourceError) Error() string {
	return fmt.Sprintf(
		"duplicate %s: %q already declared by %s at %s",
		e.KindLabel, e.Resource.Name, e.OtherKind, e.OtherLocText,
	)
}

func (e DuplicateResourceError) EventTemplate() event.Template {
	return event.Template{
		ID: CodeDuplicateResource,
		Text: `duplicate {{.KindLabel}} "{{.Resource.Name}}"` +
			` — already declared by {{.OtherKind}} at {{.OtherLocText}}`,
		Hint:   `{{.HintText}}`,
		Data:   e,
		Source: &e.Source,
	}
}

func resourceKindLabel(k spec.ResourceKind) string {
	switch k {
	case spec.ResourcePath:
		return "path"
	case spec.ResourceUser:
		return "user"
	case spec.ResourceGroup:
		return "group"
	case spec.ResourceContainer:
		return "container"
	case spec.ResourceRef:
		return "ref"
	default:
		return "resource"
	}
}

func resourceKindHint(k spec.ResourceKind) string {
	switch k {
	case spec.ResourceContainer:
		return "VMIDs are unique per cluster — change one of the IDs or merge the steps"
	case spec.ResourceUser:
		return "merge into a single posix.user step or use distinct names"
	case spec.ResourceGroup:
		return "merge into a single posix.group step or use distinct names"
	case spec.ResourcePath:
		return "two steps cannot manage the same path — merge them or pick distinct destinations"
	default:
		return "two steps cannot promise the same resource — remove or rename one"
	}
}

func formatSpan(s spec.SourceSpan) string {
	if s.Filename == "" {
		return "(unknown location)"
	}
	if s.StartLine == 0 {
		return s.Filename
	}
	return fmt.Sprintf("%s:%d", s.Filename, s.StartLine)
}
