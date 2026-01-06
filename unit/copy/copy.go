package copy

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

type (
	Copy       struct{}
	CopyConfig struct {
		Name  string
		Src   string
		Dest  string
		Perm  string
		Owner string
		Group string
	}
	copyAction struct {
		idx   int
		name  string
		src   string
		dest  string
		mode  fs.FileMode
		owner string
		group string
	}
)

func (Copy) Kind() string   { return "copy" }
func (Copy) NewConfig() any { return &CopyConfig{} }

func (Copy) Plan(idx int, config any) (spec.Action, error) {
	cfg, ok := config.(*CopyConfig)
	if !ok {
		return nil, fmt.Errorf("expected %T got %T", &CopyConfig{}, config)
	}

	mode, err := parsePerm(cfg.Perm)
	if err != nil {
		return nil, err
	}

	return &copyAction{
		idx:   idx,
		name:  cfg.Name,
		src:   cfg.Src,
		dest:  cfg.Dest,
		mode:  mode,
		owner: cfg.Owner,
		group: cfg.Group,
	}, nil
}

func (c *copyAction) Name() string {
	return fmt.Sprintf(`"%s" (copy, idx=%d)`, c.name, c.idx)
}

func (c *copyAction) Ops() []spec.Op {
	cp := &copyFileOp{
		src:  c.src,
		dest: c.dest,
	}
	chown := &ensureOwnerOp{
		path:  c.dest,
		owner: c.owner,
		group: c.group,
	}
	chmod := &ensureModeOp{
		path: c.dest,
		mode: c.mode,
	}

	action := c.Name()
	cp.setAction(action)
	chown.setAction(action)
	chmod.setAction(action)

	chown.addDependency(cp)
	chmod.addDependency(cp)

	return []spec.Op{
		cp,
		chown,
		chmod,
	}
}

type (
	baseOp struct {
		action string
		deps   []spec.Op
	}
	copyFileOp struct {
		baseOp
		src  string
		dest string
	}
	ensureOwnerOp struct {
		baseOp
		path  string
		owner string
		group string
	}
	ensureModeOp struct {
		baseOp
		path string
		mode fs.FileMode
	}
)

func (op *baseOp) Action() string            { return op.action }
func (op *baseOp) DependsOn() []spec.Op      { return op.deps }
func (op *baseOp) addDependency(dep spec.Op) { op.deps = append(op.deps, dep) }
func (op *baseOp) setAction(action string)   { op.action = action }

func (op *copyFileOp) Name() string { return "copyFileOp" }
func (op *copyFileOp) Check(ctx context.Context, tgt target.Target) (spec.CheckResult, error) {
	srcData, err := os.ReadFile(op.src)
	if err != nil {
		// fail if src file does not exist or is unreadable or whatever
		// probably better using STAT in the future, but oh well
		return spec.CheckUnsatisfied, err
	}

	destData, err := tgt.ReadFile(ctx, op.dest)
	if err != nil || !bytes.Equal(srcData, destData) {
		// we do not fail the playbook if dest doesn't exist, is unreadable, etc.
		// we're just indicating that copyFileOp needs to run
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *copyFileOp) Execute(ctx context.Context, tgt target.Target) (spec.Result, error) {
	srcData, err := os.ReadFile(op.src)
	if err != nil {
		return spec.Result{}, err
	}

	destData, err := tgt.ReadFile(ctx, op.dest)
	if err == nil && bytes.Equal(srcData, destData) {
		return spec.Result{Changed: false}, nil
	}

	if err := tgt.WriteFile(ctx, op.dest, srcData, 0o644); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (op *ensureOwnerOp) Name() string { return "ensureOwnerOp" }
func (op *ensureOwnerOp) Check(ctx context.Context, tgt target.Target) (spec.CheckResult, error) {
	haveOwner, err := tgt.GetOwner(ctx, op.path)
	if err != nil {
		return spec.CheckUnknown, err
	}

	wantOwner := target.Owner{User: op.owner, Group: op.group}
	// TODO: this is probably a target concern
	if !reflect.DeepEqual(haveOwner, wantOwner) {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureOwnerOp) Execute(ctx context.Context, tgt target.Target) (spec.Result, error) {
	if err := tgt.Chown(ctx, op.path, target.Owner{User: op.owner, Group: op.group}); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}
func (op *ensureModeOp) Name() string { return "ensureModeOp" }
func (op *ensureModeOp) Check(ctx context.Context, tgt target.Target) (spec.CheckResult, error) {
	info, err := tgt.Stat(ctx, op.path)
	if err != nil {
		return spec.CheckUnsatisfied, nil
	}

	want := info.Mode()
	if want != op.mode {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureModeOp) Execute(ctx context.Context, tgt target.Target) (spec.Result, error) {
	if err := tgt.Chmod(ctx, op.path, op.mode); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func parsePerm(s string) (fs.FileMode, error) {
	switch {
	case isOctal(s):
		return parseOctal(s)
	case isLsStyle(s):
		return parseLsStyle(s)
	case isPosixAbsolute(s):
		return parsePosixAbsolute(s)
	default:
		return fs.FileMode(0), fmt.Errorf("nah perm expr: %s", s)
	}
}

func isOctal(s string) bool {
	octalRe := regexp.MustCompile(`^0[0-7]{3}$`)
	return octalRe.MatchString(s)
}

func parseOctal(s string) (fs.FileMode, error) {
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, err
	}
	return fs.FileMode(v) & fs.ModePerm, nil
}

func isLsStyle(s string) bool {
	lsRe := regexp.MustCompile(`^[r-][w-][x-][r-][w-][x-][r-][w-][x-]$`)
	return lsRe.MatchString(s)
}

func parseLsStyle(s string) (fs.FileMode, error) {
	var mode fs.FileMode

	triples := []struct {
		offset int
		shift  uint
	}{
		{0, 6}, // user
		{3, 3}, // group
		{6, 0}, // other
	}

	for _, t := range triples {
		var bits fs.FileMode

		if s[t.offset] == 'r' {
			bits |= 4
		}
		if s[t.offset+1] == 'w' {
			bits |= 2
		}
		if s[t.offset+2] == 'x' {
			bits |= 1
		}

		mode |= bits << t.shift
	}

	return mode & fs.ModePerm, nil
}

func isPosixAbsolute(s string) bool {
	posixRe := regexp.MustCompile(`^(u|g|o)=[rwx]*(,(u|g|o)=[rwx]*)*$`)
	return posixRe.MatchString(s)
}

func parsePosixAbsolute(s string) (fs.FileMode, error) {
	seen := map[byte]bool{}
	var mode fs.FileMode

	for c := range strings.SplitSeq(s, ",") {
		if len(c) < 3 || c[1] != '=' {
			return 0, fmt.Errorf("invalid clause %q", c)
		}

		who := c[0]
		if who != 'u' && who != 'g' && who != 'o' {
			return 0, fmt.Errorf("invalid target %q", who)
		}
		if seen[who] {
			return 0, fmt.Errorf("duplicate target %q", who)
		}
		seen[who] = true

		var bits fs.FileMode
		for _, ch := range c[2:] {
			switch ch {
			case 'r':
				bits |= 4
			case 'w':
				bits |= 2
			case 'x':
				bits |= 1
			default:
				return 0, fmt.Errorf("invalid permission %q", ch)
			}
		}

		shift := map[byte]uint{'u': 6, 'g': 3, 'o': 0}[who]
		mode |= bits << shift
	}

	if len(seen) != 3 {
		return 0, fmt.Errorf("u, g, and o must all be specified")
	}

	return mode & fs.ModePerm, nil
}
