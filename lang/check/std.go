// SPDX-License-Identifier: GPL-3.0-only

package check

// TargetModule returns a scope for the std/target submodule containing
// ssh, local, and rest target steps.
func TargetModule() *Scope {
	s := NewScope(nil, ScopeFile)
	s.Define(&Symbol{Name: "ssh", Type: &DeclType{
		Name: "ssh",
		Params: []*FieldDef{
			{Name: "name", Type: StringType},
			{Name: "host", Type: StringType},
			{Name: "user", Type: StringType},
			{Name: "port", Type: IntType, HasDef: true},
			{Name: "key", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "insecure", Type: BoolType, HasDef: true},
			{Name: "timeout", Type: StringType, HasDef: true},
		},
		Ret: TargetType,
	}, Kind: SymDecl})
	s.Define(&Symbol{Name: "local", Type: &DeclType{
		Name:   "local",
		Params: []*FieldDef{{Name: "name", Type: StringType}},
		Ret:    TargetType,
	}, Kind: SymDecl})
	s.Define(&Symbol{Name: "rest", Type: &DeclType{
		Name: "rest",
		Params: []*FieldDef{
			{Name: "name", Type: StringType},
			{Name: "base_url", Type: StringType},
			{Name: "auth", Type: AuthType, HasDef: true},
			{Name: "tls", Type: TLSType, HasDef: true},
		},
		Ret: TargetType,
	}, Kind: SymDecl})
	return s
}

// StdModule returns a scope populated with the std library symbols.
// For v0, these are hardcoded. Future: generated from Go struct tags
// via the stub generator (#137).
func StdModule() *Scope {
	s := NewScope(nil, ScopeFile)

	// Scalar functions.
	s.Define(&Symbol{Name: "env", Type: &FuncType{
		Params: []*FieldDef{
			{Name: "name", Type: StringType},
			{Name: "default", Type: &Optional{Inner: StringType}, HasDef: true},
		},
		Ret: StringType,
	}, Kind: SymFunc})

	s.Define(&Symbol{Name: "secret", Type: &FuncType{
		Params: []*FieldDef{{Name: "name", Type: StringType}},
		Ret:    StringType,
	}, Kind: SymFunc})

	// Source composables.
	s.Define(&Symbol{Name: "local", Type: &DeclType{
		Name:   "local",
		Params: []*FieldDef{{Name: "path", Type: StringType}},
		Ret:    SourceType,
	}, Kind: SymDecl})

	s.Define(&Symbol{Name: "inline", Type: &DeclType{
		Name:   "inline",
		Params: []*FieldDef{{Name: "content", Type: StringType}},
		Ret:    SourceType,
	}, Kind: SymDecl})

	s.Define(&Symbol{Name: "remote", Type: &DeclType{
		Name: "remote",
		Params: []*FieldDef{
			{Name: "url", Type: StringType},
			{Name: "checksum", Type: &Optional{Inner: StringType}, HasDef: true},
		},
		Ret: SourceType,
	}, Kind: SymDecl})

	// Package sources.
	s.Define(&Symbol{Name: "system", Type: &DeclType{
		Name: "system", Ret: PkgSourceType,
	}, Kind: SymDecl})

	s.Define(&Symbol{Name: "apt_repo", Type: &DeclType{
		Name: "apt_repo",
		Params: []*FieldDef{
			{Name: "url", Type: StringType},
			{Name: "key_url", Type: StringType},
			{Name: "components", Type: &Optional{Inner: &List{Elem: StringType}}, HasDef: true},
			{Name: "suite", Type: &Optional{Inner: StringType}, HasDef: true},
		},
		Ret: PkgSourceType,
	}, Kind: SymDecl})

	s.Define(&Symbol{Name: "dnf_repo", Type: &DeclType{
		Name: "dnf_repo",
		Params: []*FieldDef{
			{Name: "url", Type: StringType},
			{Name: "key_url", Type: &Optional{Inner: StringType}, HasDef: true},
		},
		Ret: PkgSourceType,
	}, Kind: SymDecl})

	// Secrets config.
	s.Define(&Symbol{Name: "secrets", Type: &DeclType{
		Name: "secrets",
		Params: []*FieldDef{
			{Name: "backend", Type: StringType},
			{Name: "path", Type: StringType},
		},
		Ret: SecretsConfigType,
	}, Kind: SymDecl})

	// Deploy.
	s.Define(&Symbol{Name: "deploy", Type: &DeclType{
		Name: "deploy",
		Params: []*FieldDef{
			{Name: "name", Type: StringType},
			{Name: "targets", Type: &List{Elem: TargetType}},
		},
		Ret:     DeployType,
		HasBody: true,
	}, Kind: SymDecl})

	// Enums.
	pkgState := &EnumType{Name: "PkgState", Variants: []string{"present", "absent", "latest"}}
	svcState := &EnumType{Name: "SvcState", Variants: []string{"running", "stopped", "restarted", "reloaded"}}
	userState := &EnumType{Name: "UserState", Variants: []string{"present", "absent"}}
	groupState := &EnumType{Name: "GroupState", Variants: []string{"present", "absent"}}
	ctrState := &EnumType{Name: "CtrState", Variants: []string{"running", "stopped", "absent"}}
	mountState := &EnumType{Name: "MountState", Variants: []string{"mounted", "unmounted", "absent"}}
	fwAction := &EnumType{Name: "FwAction", Variants: []string{"allow", "deny", "reject"}}

	for _, e := range []*EnumType{pkgState, svcState, userState, groupState, ctrState, mountState, fwAction} {
		s.Define(&Symbol{Name: e.Name, Type: e, Kind: SymEnum})
	}

	// Desired-state steps.
	onChangeField := &FieldDef{Name: "on_change", Type: &List{Elem: StepInstanceType}, HasDef: true}
	descField := &FieldDef{Name: "desc", Type: &Optional{Inner: StringType}, HasDef: true}

	steps := []struct {
		name   string
		params []*FieldDef
	}{
		{"copy", []*FieldDef{
			{Name: "src", Type: SourceType},
			{Name: "dest", Type: StringType},
			{Name: "perm", Type: StringType},
			{Name: "owner", Type: StringType},
			{Name: "group", Type: StringType},
			{Name: "verify", Type: &Optional{Inner: StringType}, HasDef: true},
			descField, onChangeField,
		}},
		{"dir", []*FieldDef{
			{Name: "path", Type: StringType},
			{Name: "perm", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "owner", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "group", Type: &Optional{Inner: StringType}, HasDef: true},
			descField, onChangeField,
		}},
		{"symlink", []*FieldDef{
			{Name: "target", Type: StringType},
			{Name: "link", Type: StringType},
			descField, onChangeField,
		}},
		{"pkg", []*FieldDef{
			{Name: "packages", Type: &List{Elem: StringType}},
			{Name: "source", Type: PkgSourceType},
			{Name: "state", Type: pkgState, HasDef: true},
			descField, onChangeField,
		}},
		{"service", []*FieldDef{
			{Name: "name", Type: StringType},
			{Name: "state", Type: svcState, HasDef: true},
			{Name: "enabled", Type: BoolType, HasDef: true},
			descField, onChangeField,
		}},
		{"user", []*FieldDef{
			{Name: "name", Type: StringType},
			{Name: "state", Type: userState, HasDef: true},
			{Name: "shell", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "home", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "system", Type: BoolType, HasDef: true},
			{Name: "password", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "groups", Type: &List{Elem: StringType}, HasDef: true},
			descField, onChangeField,
		}},
		{"group", []*FieldDef{
			{Name: "name", Type: StringType},
			{Name: "state", Type: groupState, HasDef: true},
			{Name: "gid", Type: &Optional{Inner: IntType}, HasDef: true},
			{Name: "system", Type: BoolType, HasDef: true},
			descField, onChangeField,
		}},
		{"sysctl", []*FieldDef{
			{Name: "key", Type: StringType},
			{Name: "value", Type: StringType},
			{Name: "persist", Type: BoolType, HasDef: true},
			descField, onChangeField,
		}},
		{"mount", []*FieldDef{
			{Name: "src", Type: StringType},
			{Name: "dest", Type: StringType},
			{Name: "type", Type: &EnumType{Name: "FsType", Variants: []string{
				"nfs", "nfs4", "cifs", "ext4", "xfs", "btrfs", "tmpfs", "glusterfs", "ceph",
			}}},
			{Name: "opts", Type: StringType, HasDef: true},
			{Name: "state", Type: mountState, HasDef: true},
			descField, onChangeField,
		}},
		{"firewall", []*FieldDef{
			{Name: "port", Type: StringType},
			{Name: "action", Type: fwAction, HasDef: true},
			descField, onChangeField,
		}},
		{"run", []*FieldDef{
			{Name: "apply", Type: StringType},
			{Name: "check", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "always", Type: BoolType, HasDef: true},
			descField, onChangeField,
		}},
		{"unarchive", []*FieldDef{
			{Name: "src", Type: SourceType},
			{Name: "dest", Type: StringType},
			{Name: "depth", Type: IntType, HasDef: true},
			{Name: "owner", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "group", Type: &Optional{Inner: StringType}, HasDef: true},
			{Name: "perm", Type: &Optional{Inner: StringType}, HasDef: true},
			descField, onChangeField,
		}},
	}

	for _, st := range steps {
		s.Define(&Symbol{
			Name: st.name,
			Type: &DeclType{Name: st.name, Params: st.params, Ret: StepInstanceType},
			Kind: SymDecl,
		})
	}

	return s
}
