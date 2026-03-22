// SPDX-License-Identifier: GPL-3.0-only

package posix

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/ctrmgr"
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
		return target.ContainerInfo{}, false, fmt.Errorf("parsing container inspect: %w", err)
	}
	info.Name = name
	return info, true, nil
}

func (b Base) CreateContainer(ctx context.Context, opts target.ContainerInfo) error {
	cmd := b.CtrBackend.CmdCreate(ctrmgr.CreateOpts{
		Name:    opts.Name,
		Image:   opts.Image,
		Restart: opts.Restart,
		Ports:   opts.Ports,
		Env:     opts.Env,
		Mounts:  opts.Mounts,
		Args:    opts.Args,
		Labels:  opts.Labels,
	})
	result, err := b.runContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("create container %q: %s", opts.Name, strings.TrimSpace(result.Stderr))
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
		return fmt.Errorf("start container %q: %s", name, strings.TrimSpace(result.Stderr))
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
		return fmt.Errorf("stop container %q: %s", name, strings.TrimSpace(result.Stderr))
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
		return fmt.Errorf("remove container %q: %s", name, strings.TrimSpace(result.Stderr))
	}
	return nil
}

// parseInspect extracts ContainerInfo from docker/podman inspect JSON output.
func parseInspect(jsonStr string) (target.ContainerInfo, error) {
	var raw struct {
		Config struct {
			Image  string            `json:"Image"`
			Env    []string          `json:"Env"`
			Cmd    []string          `json:"Cmd"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Running bool `json:"Running"`
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
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
		} `json:"HostConfig"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return target.ContainerInfo{}, err
	}

	var ports []string
	for containerPort, bindings := range raw.HostConfig.PortBindings {
		// containerPort is like "9090/tcp"
		cp := strings.TrimSuffix(containerPort, "/tcp")
		cp = strings.TrimSuffix(cp, "/udp")
		for _, b := range bindings {
			if b.HostPort != "" {
				ports = append(ports, b.HostPort+":"+cp)
			}
		}
	}
	sort.Strings(ports)

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

	return target.ContainerInfo{
		Image:   raw.Config.Image,
		Running: raw.State.Running,
		Restart: raw.HostConfig.RestartPolicy.Name,
		Ports:   ports,
		Env:     env,
		Mounts:  mounts,
		Args:    raw.Config.Cmd,
		Labels:  raw.Config.Labels,
	}, nil
}
