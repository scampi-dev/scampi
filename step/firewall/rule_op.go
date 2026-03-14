// SPDX-License-Identifier: GPL-3.0-only

package firewall

import (
	"context"
	"fmt"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const ensureRuleID = "builtin.firewall.ensureRule"

type backend int

const (
	backendUFW backend = iota
	backendFirewalld
)

type ensureRuleOp struct {
	sharedops.BaseOp
	port   string
	action string
}

// Backend Detection
// -----------------------------------------------------------------------------

func detectBackend(ctx context.Context, cmdr target.Command) (backend, error) {
	if r, err := cmdr.RunCommand(ctx, "ufw version"); err == nil && r.ExitCode == 0 {
		return backendUFW, nil
	}
	if r, err := cmdr.RunCommand(ctx, "firewall-cmd --version"); err == nil && r.ExitCode == 0 {
		return backendFirewalld, nil
	}
	return 0, BackendNotFoundError{}
}

// Check
// -----------------------------------------------------------------------------

func (op *ensureRuleOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](ensureRuleID, tgt)

	be, err := detectBackend(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	switch be {
	case backendUFW:
		return op.checkUFW(ctx, cmdr)
	case backendFirewalld:
		return op.checkFirewalld(ctx, cmdr)
	default:
		return spec.CheckUnsatisfied, nil, BackendNotFoundError{}
	}
}

func (op *ensureRuleOp) checkUFW(
	ctx context.Context,
	cmdr target.Command,
) (spec.CheckResult, []spec.DriftDetail, error) {
	result, err := cmdr.RunCommand(ctx, "ufw show added")
	if err != nil {
		return spec.CheckUnsatisfied, nil, sharedops.DiagnoseTargetError(err)
	}
	if result.ExitCode != 0 {
		stderr := result.Stderr
		if stderr == "" {
			stderr = fmt.Sprintf("exit %d", result.ExitCode)
		}
		return spec.CheckUnsatisfied, nil, RuleCheckError{
			Port:   op.port,
			Stderr: stderr,
		}
	}

	needle := fmt.Sprintf("ufw %s %s", op.action, op.port)
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.TrimSpace(line) == needle {
			return spec.CheckSatisfied, nil, nil
		}
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "rule",
		Current: "(absent)",
		Desired: fmt.Sprintf("%s %s", op.action, op.port),
	}}, nil
}

func (op *ensureRuleOp) checkFirewalld(
	ctx context.Context,
	cmdr target.Command,
) (spec.CheckResult, []spec.DriftDetail, error) {
	if op.action == "allow" {
		result, err := cmdr.RunCommand(ctx, fmt.Sprintf("firewall-cmd --query-port=%s", op.port))
		if err != nil {
			return spec.CheckUnsatisfied, nil, sharedops.DiagnoseTargetError(err)
		}
		if result.ExitCode == 0 {
			return spec.CheckSatisfied, nil, nil
		}
	} else {
		richRule := op.firewalldRichRule()
		result, err := cmdr.RunCommand(ctx, fmt.Sprintf("firewall-cmd --query-rich-rule='%s'", richRule))
		if err != nil {
			return spec.CheckUnsatisfied, nil, sharedops.DiagnoseTargetError(err)
		}
		if result.ExitCode == 0 {
			return spec.CheckSatisfied, nil, nil
		}
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "rule",
		Current: "(absent)",
		Desired: fmt.Sprintf("%s %s", op.action, op.port),
	}}, nil
}

// Execute
// -----------------------------------------------------------------------------

func (op *ensureRuleOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](ensureRuleID, tgt)

	be, err := detectBackend(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	switch be {
	case backendUFW:
		return op.executeUFW(ctx, cmdr)
	case backendFirewalld:
		return op.executeFirewalld(ctx, cmdr)
	default:
		return spec.Result{}, BackendNotFoundError{}
	}
}

func (op *ensureRuleOp) executeUFW(
	ctx context.Context,
	cmdr target.Command,
) (spec.Result, error) {
	cmd := fmt.Sprintf("ufw %s %s", op.action, op.port)
	result, err := cmdr.RunCommand(ctx, cmd)
	if err != nil {
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}
	if result.ExitCode != 0 {
		stderr := result.Stderr
		if stderr == "" {
			stderr = fmt.Sprintf("exit %d", result.ExitCode)
		}
		return spec.Result{}, RuleApplyError{
			Port:   op.port,
			Action: op.action,
			Stderr: stderr,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (op *ensureRuleOp) executeFirewalld(
	ctx context.Context,
	cmdr target.Command,
) (spec.Result, error) {
	if op.action == "allow" {
		cmd := fmt.Sprintf("firewall-cmd --permanent --add-port=%s", op.port)
		result, err := cmdr.RunCommand(ctx, cmd)
		if err != nil {
			return spec.Result{}, sharedops.DiagnoseTargetError(err)
		}
		if result.ExitCode != 0 {
			return spec.Result{}, op.applyError(result)
		}
	} else {
		richRule := op.firewalldRichRule()
		cmd := fmt.Sprintf("firewall-cmd --permanent --add-rich-rule='%s'", richRule)
		result, err := cmdr.RunCommand(ctx, cmd)
		if err != nil {
			return spec.Result{}, sharedops.DiagnoseTargetError(err)
		}
		if result.ExitCode != 0 {
			return spec.Result{}, op.applyError(result)
		}
	}

	reload, err := cmdr.RunCommand(ctx, "firewall-cmd --reload")
	if err != nil {
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}
	if reload.ExitCode != 0 {
		return spec.Result{}, RuleApplyError{
			Port:   op.port,
			Action: op.action,
			Stderr: "reload failed: " + reload.Stderr,
		}
	}

	return spec.Result{Changed: true}, nil
}

// Helpers
// -----------------------------------------------------------------------------

// firewalldRichRule builds a rich rule for deny/reject actions.
// Port string is expected to be validated as <port>/<proto> or <start>:<end>/<proto>.
func (op *ensureRuleOp) firewalldRichRule() string {
	parts := strings.SplitN(op.port, "/", 2)
	port, proto := parts[0], parts[1]

	verb := "reject"
	if op.action == "deny" {
		verb = "drop"
	}

	return fmt.Sprintf("rule port port=%s protocol=%s %s", port, proto, verb)
}

func (op *ensureRuleOp) applyError(result target.CommandResult) RuleApplyError {
	stderr := result.Stderr
	if stderr == "" {
		stderr = fmt.Sprintf("exit %d", result.ExitCode)
	}
	return RuleApplyError{
		Port:   op.port,
		Action: op.action,
		Stderr: stderr,
	}
}

func (ensureRuleOp) RequiredCapabilities() capability.Capability {
	return capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type ensureRuleDesc struct {
	Action string
	Port   string
}

func (d ensureRuleDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureRuleID,
		Text: `firewall {{.Action}} {{.Port}}`,
		Data: d,
	}
}

func (op *ensureRuleOp) OpDescription() spec.OpDescription {
	return ensureRuleDesc{Action: op.action, Port: op.port}
}
