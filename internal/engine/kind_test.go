// SPDX-License-Identifier: GPL-3.0-only

package engine

import "testing"

func TestKindSchemas_WellFormed(t *testing.T) {
	commonNames := map[string]bool{}
	for _, spec := range commonAttrs {
		commonNames[spec.Name] = true
	}
	for name, k := range kinds {
		t.Run(name, func(t *testing.T) {
			sch := k.Schema()
			seen := map[string]bool{}
			for _, spec := range sch {
				if spec.Name == "" {
					t.Error("empty attr name")
				}
				if seen[spec.Name] {
					t.Errorf("attr %q declared twice", spec.Name)
				}
				seen[spec.Name] = true
				if commonNames[spec.Name] {
					t.Errorf("attr %q shadows engine-common attr", spec.Name)
				}
				if spec.Required && spec.Default != (Value{}) {
					t.Errorf("attr %q: Required must not carry a Default", spec.Name)
				}
				if !spec.Required && spec.Default.Kind != spec.Type {
					t.Errorf("attr %q: optional Default kind %v must match Type %v",
						spec.Name, spec.Default.Kind, spec.Type)
				}
			}
			for _, idName := range k.Identify() {
				spec := sch.Find(idName)
				if spec == nil {
					t.Errorf("identity %q not in schema", idName)
					continue
				}
				if spec.Type != ValueString {
					t.Errorf("identity %q must be string, got %v", idName, spec.Type)
				}
			}
		})
	}
}
