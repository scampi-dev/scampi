// SPDX-License-Identifier: GPL-3.0-only

package posix

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/ctrmgr"
)

func (b Base) runContainer(ctx context.Context, cmd string) (target.CommandResult, error) {
	if b.CtrBackend.NeedsRoot {
		return b.RunPrivileged(ctx, cmd)
	}
	return b.Runner(ctx, cmd)
}

func (b Base) InspectContainer(ctx context.Context, name string) (target.ContainerInfo, bool, error) {
	cmd := b.CtrBackend.CmdInspect(name)
	result, err := b.runContainer(ctx, cmd)
	if err != nil {
		return target.ContainerInfo{}, false, err
	}
	if result.ExitCode != 0 {
		return target.ContainerInfo{}, false, nil
	}
	info, err := parseInspect(result.Stdout)
	if err != nil {
		// bare-error: container runtime error, wrapped by step before reaching engine
		return target.ContainerInfo{}, false, errs.Errorf("parsing container inspect: %w", err)
	}
	info.Name = name
	return info, true, nil
}

func (b Base) CreateContainer(ctx context.Context, opts target.ContainerInfo) error {
	cmd := b.CtrBackend.CmdCreate(ctrmgr.CreateOpts{
		Name:        opts.Name,
		Image:       opts.Image,
		Restart:     opts.Restart,
		Ports:       opts.Ports,
		Env:         opts.Env,
		Mounts:      opts.Mounts,
		Args:        opts.Args,
		Labels:      opts.Labels,
		Healthcheck: opts.Healthcheck,
	})
	result, err := b.runContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: container runtime error, wrapped by step before reaching engine
		return errs.Errorf("create container %q: %s", opts.Name, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func (b Base) StartContainer(ctx context.Context, name string) error {
	cmd := b.CtrBackend.CmdStart(name)
	result, err := b.runContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: container runtime error, wrapped by step before reaching engine
		return errs.Errorf("start container %q: %s", name, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func (b Base) StopContainer(ctx context.Context, name string) error {
	cmd := b.CtrBackend.CmdStop(name)
	result, err := b.runContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: container runtime error, wrapped by step before reaching engine
		return errs.Errorf("stop container %q: %s", name, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func (b Base) RemoveContainer(ctx context.Context, name string) error {
	cmd := b.CtrBackend.CmdRm(name)
	result, err := b.runContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		// bare-error: container runtime error, wrapped by step before reaching engine
		return errs.Errorf("remove container %q: %s", name, strings.TrimSpace(result.Stderr))
	}
	return nil
}

// parseInspect extracts ContainerInfo from docker/podman inspect JSON output.
func parseInspect(jsonStr string) (target.ContainerInfo, error) {
	var raw struct {
		Config struct {
			Image       string            `json:"Image"`
			Env         []string          `json:"Env"`
			Cmd         []string          `json:"Cmd"`
			Labels      map[string]string `json:"Labels"`
			Healthcheck *struct {
				Test        []string `json:"Test"`
				Interval    int64    `json:"Interval"`
				Timeout     int64    `json:"Timeout"`
				Retries     int      `json:"Retries"`
				StartPeriod int64    `json:"StartPeriod"`
			} `json:"Healthcheck"`
		} `json:"Config"`
		State struct {
			Running bool `json:"Running"`
			Health  *struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
		HostConfig struct {
			RestartPolicy struct {
				Name string `json:"Name"`
			} `json:"RestartPolicy"`
			PortBindings map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
		} `json:"HostConfig"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return target.ContainerInfo{}, err
	}

	var ports []target.Port
	for containerPort, bindings := range raw.HostConfig.PortBindings {
		// containerPort is like "9090/tcp" or "3000/udp"
		cPort, proto, _ := strings.Cut(containerPort, "/")
		for _, b := range bindings {
			if b.HostPort != "" {
				ports = append(ports, target.Port{
					HostIP:        b.HostIP,
					HostPort:      b.HostPort,
					ContainerPort: cPort,
					Proto:         target.ParsePortProto(proto),
				})
			}
		}
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].String() < ports[j].String() })

	env := make(map[string]string, len(raw.Config.Env))
	for _, entry := range raw.Config.Env {
		if k, v, ok := strings.Cut(entry, "="); ok {
			env[k] = v
		}
	}

	var mounts []target.Mount
	for _, m := range raw.Mounts {
		if m.Type != "bind" {
			continue
		}
		mounts = append(mounts, target.Mount{
			Source:   m.Source,
			Target:   m.Destination,
			ReadOnly: !m.RW,
		})
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Source < mounts[j].Source })

	var hc *target.Healthcheck
	if raw.Config.Healthcheck != nil && len(raw.Config.Healthcheck.Test) >= 2 {
		// Test is ["CMD-SHELL", "cmd"] or ["CMD", "arg0", "arg1", ...]
		var cmd string
		switch raw.Config.Healthcheck.Test[0] {
		case "CMD-SHELL":
			cmd = raw.Config.Healthcheck.Test[1]
		case "CMD":
			cmd = strings.Join(raw.Config.Healthcheck.Test[1:], " ")
		}
		hc = &target.Healthcheck{
			Cmd:         cmd,
			Interval:    time.Duration(raw.Config.Healthcheck.Interval),
			Timeout:     time.Duration(raw.Config.Healthcheck.Timeout),
			Retries:     raw.Config.Healthcheck.Retries,
			StartPeriod: time.Duration(raw.Config.Healthcheck.StartPeriod),
		}
	}

	var healthStatus string
	if raw.State.Health != nil {
		healthStatus = raw.State.Health.Status
	}

	return target.ContainerInfo{
		Image:        raw.Config.Image,
		Running:      raw.State.Running,
		Restart:      raw.HostConfig.RestartPolicy.Name,
		Ports:        ports,
		Env:          env,
		Mounts:       mounts,
		Args:         raw.Config.Cmd,
		Labels:       raw.Config.Labels,
		Healthcheck:  hc,
		HealthStatus: healthStatus,
	}, nil
}
