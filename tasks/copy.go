package tasks

import (
	"context"
	"fmt"
	"time"

	"godoit.dev/doit/spec"
)

type (
	CopySpec   struct{}
	CopyConfig struct {
		Src   string
		Dest  string
		Mode  string
		Owner string
		Group string
	}
	copyRtTask struct {
		idx int
		cfg CopyConfig
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
      mode: string
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

	return &copyRtTask{
		idx: idx,
		cfg: *cfg,
	}, nil
}

func (c *copyRtTask) Name() string {
	return fmt.Sprintf("copy[%d]", c.idx)
}

func (c *copyRtTask) Ops() []spec.Op {
	cp := &copyFileOp{
		src:  c.cfg.Src,
		dest: c.cfg.Dest,
	}
	chown := &ensureOwnerOp{
		path:  c.cfg.Dest,
		owner: c.cfg.Owner,
		group: c.cfg.Group,
	}
	chmod := &ensureModeOp{
		path: c.cfg.Dest,
		mode: c.cfg.Group,
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
		mode string
	}
)

func (op *baseOp) DependsOn() []spec.Op      { return op.deps }
func (op *baseOp) addDependency(dep spec.Op) { op.deps = append(op.deps, dep) }

func (op *copyFileOp) Name() string { return "copyFileOp" }
func (op *copyFileOp) Check(context.Context) (spec.CheckResult, error) {
	fmt.Printf("[%s] enter(op): check %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): check %T\n", time.Now().Format(time.RFC3339), op) }()
	time.Sleep(time.Second)
	return spec.CheckSatisfied, nil
}

func (op *copyFileOp) Execute(context.Context) (spec.Result, error) {
	fmt.Printf("[%s] enter(op): exec %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): exec %T\n", time.Now().Format(time.RFC3339), op) }()
	time.Sleep(time.Second)
	return spec.Result{}, nil
}
func (op *ensureOwnerOp) Name() string { return "ensureOwnerOp" }
func (op *ensureOwnerOp) Check(context.Context) (spec.CheckResult, error) {
	fmt.Printf("[%s] enter(op): check %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): check %T\n", time.Now().Format(time.RFC3339), op) }()
	time.Sleep(time.Second)
	return spec.CheckSatisfied, nil
}

func (op *ensureOwnerOp) Execute(context.Context) (spec.Result, error) {
	fmt.Printf("[%s] enter(op): exec %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): exec %T\n", time.Now().Format(time.RFC3339), op) }()
	time.Sleep(time.Second)
	return spec.Result{}, nil
}
func (op *ensureModeOp) Name() string { return "ensureModeOp" }
func (op *ensureModeOp) Check(context.Context) (spec.CheckResult, error) {
	fmt.Printf("[%s] enter(op): check %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): check %T\n", time.Now().Format(time.RFC3339), op) }()
	time.Sleep(time.Second)

	return spec.CheckSatisfied, nil
}

func (op *ensureModeOp) Execute(context.Context) (spec.Result, error) {
	fmt.Printf("[%s] enter(op): exec %T\n", time.Now().Format(time.RFC3339), op)
	defer func() { fmt.Printf("[%s] exit(op): exec %T\n", time.Now().Format(time.RFC3339), op) }()
	time.Sleep(time.Second)
	return spec.Result{}, nil
}
