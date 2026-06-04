// SPDX-License-Identifier: GPL-3.0-only

package engine

import "testing"

func TestKindSchemas_WellFormed(t *testing.T) {
	for name, k := range kinds {
		t.Run(name, func(t *testing.T) {
			sch := k.Schema()
			seen := map[string]string{}
			for _, n := range sch.Required {
				if n == "" {
					t.Error("empty name in Required")
				}
				if prev, dup := seen[n]; dup {
					t.Errorf("attr %q duplicated; already in %s", n, prev)
				}
				seen[n] = "Required"
			}
			for _, n := range sch.Optional {
				if n == "" {
					t.Error("empty name in Optional")
				}
				if prev, dup := seen[n]; dup {
					t.Errorf("attr %q duplicated; already in %s", n, prev)
				}
				seen[n] = "Optional"
			}
			if _, banned := seen["adopt"]; banned {
				t.Error(`"adopt" must not appear in any Kind schema`)
			}
		})
	}
}
