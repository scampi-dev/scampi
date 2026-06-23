// SPDX-License-Identifier: GPL-3.0-only

package posix

import (
	"context"

	"scampi.dev/scampi/internal/target"
)

// ServiceManager
// -----------------------------------------------------------------------------

func (b Base) IsActive(ctx context.Context, name string) (bool, error) {
	cmd := b.SvcBackend.CmdIsActive(name)
	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (b Base) IsEnabled(ctx context.Context, name string) (bool, error) {
	cmd := b.SvcBackend.CmdIsEnabled(name)
	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (b Base) Start(ctx context.Context, name string) error {
	return b.runSvcCommand(ctx, b.SvcBackend.CmdStart(name), "start")
}

func (b Base) Stop(ctx context.Context, name string) error {
	return b.runSvcCommand(ctx, b.SvcBackend.CmdStop(name), "stop")
}

func (b Base) Enable(ctx context.Context, name string) error {
	return b.runSvcCommand(ctx, b.SvcBackend.CmdEnable(name), "enable")
}

func (b Base) Disable(ctx context.Context, name string) error {
	return b.runSvcCommand(ctx, b.SvcBackend.CmdDisable(name), "disable")
}

func (b Base) Restart(ctx context.Context, name string) error {
	return b.runSvcCommand(ctx, b.SvcBackend.CmdRestart(name), "restart")
}

func (b Base) Reload(ctx context.Context, name string) error {
	return b.runSvcCommand(ctx, b.SvcBackend.CmdReload(name), "reload")
}

func (b Base) SupportsReload() bool {
	return b.SvcBackend.CmdReload("_") != ""
}

func (b Base) DaemonReload(ctx context.Context) error {
	cmd := b.SvcBackend.CmdDaemonReload()
	if cmd == "" {
		return nil
	}
	if b.SvcBackend.NeedsRoot() && b.NeedsEscalation() {
		return b.NoEscalation(b.SvcBackend.Name()+" daemon-reload", "")
	}
	if b.SvcBackend.NeedsRoot() && b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	result, err := b.Runner(ctx, cmd)
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

func (b Base) runSvcCommand(ctx context.Context, cmd, op string) error {
	if b.SvcBackend.NeedsRoot() && b.NeedsEscalation() {
		return b.NoEscalation(b.SvcBackend.Name()+" "+op, "")
	}
	if b.SvcBackend.NeedsRoot() && b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	result, err := b.Runner(ctx, cmd)
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
