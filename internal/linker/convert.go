// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"strings"

	"scampi.dev/scampi/internal/target"
)

// convertPort parses a port string like "8080:80" or "127.0.0.1:8080:80/udp".
func convertPort(s string) target.Port {
	p := target.Port{Proto: target.ProtoTCP}
	if base, proto, ok := strings.Cut(s, "/"); ok {
		s = base
		p.Proto = target.ParsePortProto(proto)
	}
	parts := strings.SplitN(s, ":", 3)
	switch len(parts) {
	case 2:
		p.HostPort = parts[0]
		p.ContainerPort = parts[1]
	case 3:
		p.HostIP = parts[0]
		p.HostPort = parts[1]
		p.ContainerPort = parts[2]
	default:
		p.ContainerPort = s
	}
	return p
}

// convertMount parses a mount string like "/host:/container" or "/host:/container:ro".
func convertMount(s string) target.Mount {
	m := target.Mount{}
	parts := strings.SplitN(s, ":", 3)
	if len(parts) >= 2 {
		m.Source = parts[0]
		m.Target = parts[1]
	}
	if len(parts) == 3 && parts[2] == "ro" {
		m.ReadOnly = true
	}
	return m
}
