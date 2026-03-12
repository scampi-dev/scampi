// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"context"

	"scampi.dev/scampi/target"
)

func (t POSIXTarget) IsActive(ctx context.Context, name string) (bool, error) {
	cmd := t.svcBackend.CmdIsActive(name)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (t POSIXTarget) IsEnabled(ctx context.Context, name string) (bool, error) {
	cmd := t.svcBackend.CmdIsEnabled(name)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (t POSIXTarget) Start(ctx context.Context, name string) error {
	return t.runSvcCommand(ctx, t.svcBackend.CmdStart(name), "start")
}

func (t POSIXTarget) Stop(ctx context.Context, name string) error {
	return t.runSvcCommand(ctx, t.svcBackend.CmdStop(name), "stop")
}

func (t POSIXTarget) Enable(ctx context.Context, name string) error {
	return t.runSvcCommand(ctx, t.svcBackend.CmdEnable(name), "enable")
}

func (t POSIXTarget) Disable(ctx context.Context, name string) error {
	return t.runSvcCommand(ctx, t.svcBackend.CmdDisable(name), "disable")
}

func (t POSIXTarget) Restart(ctx context.Context, name string) error {
	return t.runSvcCommand(ctx, t.svcBackend.CmdRestart(name), "restart")
}

func (t POSIXTarget) Reload(ctx context.Context, name string) error {
	return t.runSvcCommand(ctx, t.svcBackend.CmdReload(name), "reload")
}

func (t POSIXTarget) SupportsReload() bool {
	return t.svcBackend.CmdReload("_") != ""
}

func (t POSIXTarget) DaemonReload(ctx context.Context) error {
	cmd := t.svcBackend.CmdDaemonReload()
	if cmd == "" {
		return nil
	}
	if t.svcBackend.NeedsRoot() && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.svcBackend.Name() + " daemon-reload"}
	}
	if t.svcBackend.NeedsRoot() && t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.SvcCommandError{
			Op:       "daemon-reload",
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t POSIXTarget) runSvcCommand(ctx context.Context, cmd, op string) error {
	if t.svcBackend.NeedsRoot() && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.svcBackend.Name() + " " + op}
	}
	if t.svcBackend.NeedsRoot() && t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.SvcCommandError{
			Op:       op,
			Name:     cmd,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}
	}
	return nil
}
