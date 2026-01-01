package tasks

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"godoit.dev/doit/spec"
)

type (
	CopySpec   struct{}
	CopyConfig struct {
		Src   string
		Dest  string
		Perm  string
		Owner string
		Group string
	}
	copyRtTask struct {
		idx   int
		src   string
		dest  string
		mode  fs.FileMode
		owner string
		group string
	}
)

func (CopySpec) Kind() string { return "copy" }
func (CopySpec) Schema() string {
	return `
package doit

#Task: {
  kind: string

  if kind == "copy" {
    copy: {
      src: string
      dest: string
      perm: string
      owner: string
      group: string
    }
  }
}`
}

func (CopySpec) NewConfig() any {
	return &CopyConfig{}
}

func (CopySpec) Plan(idx int, config any) (spec.RtTask, error) {
	cfg, ok := config.(*CopyConfig)
	if !ok {
		return nil, fmt.Errorf("expected %T got %T", &CopyConfig{}, config)
	}

	mode, err := parsePerm(cfg.Perm)
	if err != nil {
		return nil, err
	}

	return &copyRtTask{
		idx:   idx,
		src:   cfg.Src,
		dest:  cfg.Dest,
		mode:  mode,
		owner: cfg.Owner,
		group: cfg.Group,
	}, nil
}

func (c *copyRtTask) Name() string {
	return fmt.Sprintf("copy[%d]", c.idx)
}

func (c *copyRtTask) Ops() []spec.Op {
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
		deps []spec.Op
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

func (op *baseOp) DependsOn() []spec.Op      { return op.deps }
func (op *baseOp) addDependency(dep spec.Op) { op.deps = append(op.deps, dep) }

func (op *copyFileOp) Name() string { return "copyFileOp" }
func (op *copyFileOp) Check(context.Context) (spec.CheckResult, error) {
	fmt.Printf("[%s] enter(op): check %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): check %T\n", time.Now().Format(time.RFC3339), op) }()

	srcData, err := os.ReadFile(op.src)
	if err != nil {
		// fail if src file does not exist or is unreadable or whatever
		// probably better using STAT in the future, but oh well
		return spec.CheckUnsatisfied, err
	}

	destData, err := os.ReadFile(op.dest)
	if err != nil || !bytes.Equal(srcData, destData) {
		// we do not fail the playbook if dest doesn't exist, is unreadable, etc.
		// we're just indicating that copyFileOp needs to run
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *copyFileOp) Execute(context.Context) (spec.Result, error) {
	fmt.Printf("[%s] enter(op): exec %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): exec %T\n", time.Now().Format(time.RFC3339), op) }()

	srcData, err := os.ReadFile(op.src)
	if err != nil {
		return spec.Result{}, err
	}

	destData, err := os.ReadFile(op.dest)
	if err == nil && bytes.Equal(srcData, destData) {
		return spec.Result{Changed: false}, nil
	}

	if err := os.WriteFile(op.dest, srcData, 0o644); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (op *ensureOwnerOp) Name() string { return "ensureOwnerOp" }
func (op *ensureOwnerOp) Check(context.Context) (spec.CheckResult, error) {
	fmt.Printf("[%s] enter(op): check %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): check %T\n", time.Now().Format(time.RFC3339), op) }()

	info, err := os.Stat(op.path)
	if err != nil {
		// file might not exist yet or whatever
		// don't fail, just signal that we need to run
		return spec.CheckUnsatisfied, nil
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return spec.CheckUnknown, fmt.Errorf("expected %T got %T", &syscall.Stat_t{}, info.Sys())
	}

	haveUsr, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return spec.CheckUnknown, err
	}
	haveGrp, err := user.LookupGroupId(strconv.FormatUint(uint64(stat.Gid), 10))
	if err != nil {
		return spec.CheckUnknown, err
	}

	wantUsr, err := lookupUser(op.owner)
	if err != nil {
		return spec.CheckUnknown, err
	}

	wantGrp, err := lookupGroup(op.group)
	if err != nil {
		return spec.CheckUnknown, err
	}

	if haveUsr.Uid != wantUsr.Uid || haveGrp.Gid != wantGrp.Gid {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureOwnerOp) Execute(context.Context) (spec.Result, error) {
	fmt.Printf("[%s] enter(op): exec %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): exec %T\n", time.Now().Format(time.RFC3339), op) }()

	usr, err := lookupUser(op.owner)
	if err != nil {
		return spec.Result{}, err
	}

	grp, err := lookupGroup(op.group)
	if err != nil {
		return spec.Result{}, err
	}

	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return spec.Result{}, err
	}

	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return spec.Result{}, err
	}

	if err := os.Chown(op.path, uid, gid); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}
func (op *ensureModeOp) Name() string { return "ensureModeOp" }
func (op *ensureModeOp) Check(context.Context) (spec.CheckResult, error) {
	fmt.Printf("[%s] enter(op): check %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): check %T\n", time.Now().Format(time.RFC3339), op) }()

	info, err := os.Stat(op.path)
	if err != nil {
		return spec.CheckUnsatisfied, nil
	}

	want := info.Mode()
	if want != op.mode {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureModeOp) Execute(context.Context) (spec.Result, error) {
	fmt.Printf("[%s] enter(op): exec %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): exec %T\n", time.Now().Format(time.RFC3339), op) }()

	if err := os.Chmod(op.path, op.mode); err != nil {
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
	fmt.Printf("parse octal: %s\n", s)
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
	fmt.Printf("parse ls: %s\n", s)

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
	fmt.Printf("parse POSIX: %s\n", s)

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

func lookupUser(u string) (*user.User, error) {
	if isLikelyID(u) {
		return user.LookupId(u)
	}
	return user.Lookup(u)
}

func lookupGroup(g string) (*user.Group, error) {
	if isLikelyID(g) {
		return user.LookupGroupId(g)
	}
	return user.LookupGroup(g)
}

func isLikelyID(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
