// SPDX-License-Identifier: GPL-3.0-only

package ctrmgr

import (
	"fmt"
	"sort"
	"strings"

	"scampi.dev/scampi/internal/target"
)

// Backend builds shell commands for a container runtime.
type Backend struct {
	name      string
	NeedsRoot bool
}

func (b *Backend) Name() string { return b.name }

func (b *Backend) CmdInspect(name string) string {
	return fmt.Sprintf("%s inspect --format '{{json .}}' %s", b.name, target.ShellQuote(name))
}

func (b *Backend) CmdCreate(opts CreateOpts) string {
	parts := []string{
		b.name, "create",
		"--name", target.ShellQuote(opts.Name),
		"--restart", target.ShellQuote(opts.Restart),
	}
	for _, p := range opts.Ports {
		parts = append(parts, "-p", target.ShellQuote(p.Flag()))
	}
	for _, m := range opts.Mounts {
		flag := "type=bind,source=" + m.Source + ",target=" + m.Target
		if m.ReadOnly {
			flag += ",readonly"
		}
		parts = append(parts, "--mount", target.ShellQuote(flag))
	}
	if len(opts.Env) > 0 {
		keys := make([]string, 0, len(opts.Env))
		for k := range opts.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, "--env", target.ShellQuote(k+"="+opts.Env[k]))
		}
	}
	if len(opts.Labels) > 0 {
		keys := make([]string, 0, len(opts.Labels))
		for k := range opts.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, "--label", target.ShellQuote(k+"="+opts.Labels[k]))
		}
	}
	if opts.Healthcheck != nil {
		hc := opts.Healthcheck
		parts = append(parts,
			"--health-cmd", target.ShellQuote(hc.Cmd),
			"--health-interval", hc.Interval.String(),
			"--health-timeout", hc.Timeout.String(),
			"--health-retries", fmt.Sprintf("%d", hc.Retries),
			"--health-start-period", hc.StartPeriod.String(),
		)
	}
	parts = append(parts, target.ShellQuote(opts.Image))
	for _, a := range opts.Args {
		parts = append(parts, target.ShellQuote(a))
	}
	return strings.Join(parts, " ")
}

func (b *Backend) CmdStart(name string) string {
	return fmt.Sprintf("%s start %s", b.name, target.ShellQuote(name))
}

func (b *Backend) CmdStop(name string) string {
	return fmt.Sprintf("%s stop %s", b.name, target.ShellQuote(name))
}

func (b *Backend) CmdRm(name string) string {
	return fmt.Sprintf("%s rm %s", b.name, target.ShellQuote(name))
}

func (b *Backend) CmdPull(image string) string {
	return fmt.Sprintf("%s pull %s", b.name, target.ShellQuote(image))
}

// CreateOpts holds parameters for creating a container.
type CreateOpts struct {
	Name        string
	Image       string
	Restart     string
	Ports       []target.Port
	Env         map[string]string
	Mounts      []target.Mount
	Args        []string
	Labels      map[string]string
	Healthcheck *target.Healthcheck
}
