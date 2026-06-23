// SPDX-License-Identifier: GPL-3.0-only

package posix

import (
	"context"
	"strconv"
	"strings"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/target"
)

// UserManager
// -----------------------------------------------------------------------------

func (b Base) UserExists(ctx context.Context, name string) (bool, error) {
	result, err := b.Runner(ctx, "getent passwd "+target.ShellQuote(name))
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (b Base) GetUser(ctx context.Context, name string) (target.UserInfo, error) {
	result, err := b.Runner(ctx, "getent passwd "+target.ShellQuote(name))
	if err != nil {
		return target.UserInfo{}, err
	}
	if result.ExitCode != 0 {
		return target.UserInfo{}, target.ErrUnknownUser
	}
	info, err := ParsePasswdLine(strings.TrimSpace(result.Stdout))
	if err != nil {
		return target.UserInfo{}, err
	}

	grResult, err := b.Runner(ctx, "id -Gn "+target.ShellQuote(name))
	if err == nil && grResult.ExitCode == 0 {
		allGroups := strings.Fields(strings.TrimSpace(grResult.Stdout))
		pgResult, _ := b.Runner(ctx, "id -gn "+target.ShellQuote(name))
		primaryGroup := strings.TrimSpace(pgResult.Stdout)
		var supplementary []string
		for _, g := range allGroups {
			if g != primaryGroup {
				supplementary = append(supplementary, g)
			}
		}
		info.Groups = supplementary
	}

	return info, nil
}

func (b Base) CreateUser(ctx context.Context, info target.UserInfo) error {
	if b.NeedsEscalation() {
		return b.NoEscalation("useradd", "")
	}

	cmd := "useradd"
	if info.Shell != "" {
		cmd += " -s " + target.ShellQuote(info.Shell)
	}
	if info.Home != "" {
		cmd += " -m -d " + target.ShellQuote(info.Home)
	}
	if info.System {
		cmd += " -r"
	}
	if info.Password != "" {
		cmd += " -p " + target.ShellQuote(info.Password)
	}
	if len(info.Groups) > 0 {
		cmd += " -G " + target.ShellQuote(strings.Join(info.Groups, ","))
	}
	cmd += " " + target.ShellQuote(info.Name)

	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}

	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: system command error, wrapped by step before reaching engine
		return errs.Errorf("useradd %s failed (exit %d): %s", info.Name, result.ExitCode, result.Stderr)
	}
	// useradd on many distros also creates a primary group with the
	// same name (USERGROUPS_ENAB yes), so invalidate both namespaces.
	b.Identity.InvalidateUser(info.Name)
	b.Identity.InvalidateGroup(info.Name)
	return nil
}

func (b Base) ModifyUser(ctx context.Context, info target.UserInfo) error {
	if b.NeedsEscalation() {
		return b.NoEscalation("usermod", "")
	}

	cmd := "usermod"
	if info.Shell != "" {
		cmd += " -s " + target.ShellQuote(info.Shell)
	}
	if info.Home != "" {
		cmd += " -d " + target.ShellQuote(info.Home)
	}
	if info.Password != "" {
		cmd += " -p " + target.ShellQuote(info.Password)
	}
	if info.Groups != nil {
		cmd += " -G " + target.ShellQuote(strings.Join(info.Groups, ","))
	}
	cmd += " " + target.ShellQuote(info.Name)

	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}

	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: system command error, wrapped by step before reaching engine
		return errs.Errorf("usermod %s failed (exit %d): %s", info.Name, result.ExitCode, result.Stderr)
	}
	b.Identity.InvalidateUser(info.Name)
	return nil
}

func (b Base) DeleteUser(ctx context.Context, name string) error {
	if b.NeedsEscalation() {
		return b.NoEscalation("userdel", "")
	}

	cmd := "userdel " + target.ShellQuote(name)
	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}

	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: system command error, wrapped by step before reaching engine
		return errs.Errorf("userdel %s failed (exit %d): %s", name, result.ExitCode, result.Stderr)
	}
	// userdel of a user with USERGROUPS_ENAB also removes the primary
	// group of the same name when empty; invalidate both.
	b.Identity.InvalidateUser(name)
	b.Identity.InvalidateGroup(name)
	return nil
}

// GroupManager
// -----------------------------------------------------------------------------

func (b Base) GroupExists(ctx context.Context, name string) (bool, error) {
	result, err := b.Runner(ctx, "getent group "+target.ShellQuote(name))
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (b Base) GetGroup(ctx context.Context, name string) (target.GroupInfo, error) {
	result, err := b.Runner(ctx, "getent group "+target.ShellQuote(name))
	if err != nil {
		return target.GroupInfo{}, err
	}
	if result.ExitCode != 0 {
		return target.GroupInfo{}, target.ErrUnknownGroup
	}
	return ParseGroupLine(strings.TrimSpace(result.Stdout))
}

func (b Base) CreateGroup(ctx context.Context, info target.GroupInfo) error {
	if b.NeedsEscalation() {
		return b.NoEscalation("groupadd", "")
	}

	cmd := "groupadd"
	if info.GID != 0 {
		cmd += " -g " + strconv.Itoa(info.GID)
	}
	if info.System {
		cmd += " -r"
	}
	cmd += " " + target.ShellQuote(info.Name)

	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}

	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: system command error, wrapped by step before reaching engine
		return errs.Errorf("groupadd %s failed (exit %d): %s", info.Name, result.ExitCode, result.Stderr)
	}
	b.Identity.InvalidateGroup(info.Name)
	return nil
}

func (b Base) DeleteGroup(ctx context.Context, name string) error {
	if b.NeedsEscalation() {
		return b.NoEscalation("groupdel", "")
	}

	cmd := "groupdel " + target.ShellQuote(name)
	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}

	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: system command error, wrapped by step before reaching engine
		return errs.Errorf("groupdel %s failed (exit %d): %s", name, result.ExitCode, result.Stderr)
	}
	b.Identity.InvalidateGroup(name)
	return nil
}

// Helpers
// -----------------------------------------------------------------------------

// ParsePasswdLine parses a getent passwd line: name:x:uid:gid:gecos:home:shell
func ParsePasswdLine(line string) (target.UserInfo, error) {
	parts := strings.Split(line, ":")
	if len(parts) < 7 {
		// bare-error: parse error, wrapped by step before reaching engine
		return target.UserInfo{}, errs.Errorf("unexpected passwd format: %q", line)
	}
	uid, _ := strconv.Atoi(parts[2])
	gid, _ := strconv.Atoi(parts[3])
	return target.UserInfo{
		Name:  parts[0],
		UID:   uid,
		GID:   gid,
		Home:  parts[5],
		Shell: parts[6],
	}, nil
}

// ParseGroupLine parses a getent group line: name:x:gid:member1,member2
func ParseGroupLine(line string) (target.GroupInfo, error) {
	parts := strings.Split(line, ":")
	if len(parts) < 4 {
		// bare-error: parse error, wrapped by step before reaching engine
		return target.GroupInfo{}, errs.Errorf("unexpected group format: %q", line)
	}
	gid, _ := strconv.Atoi(parts[2])
	var members []string
	if parts[3] != "" {
		members = strings.Split(parts[3], ",")
	}
	return target.GroupInfo{
		Name:    parts[0],
		GID:     gid,
		Members: members,
	}, nil
}
