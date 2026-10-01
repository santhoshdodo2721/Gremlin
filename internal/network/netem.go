package network

import (
"context"
"fmt"
"strings"

"gremlin-in-a-box/internal/dockerapi"
)

type Manager struct {
docker     *dockerapi.Client
interface_ string
}

func NewManager(docker *dockerapi.Client, iface string) *Manager {
if iface == "" {
iface = "eth0"
}
return &Manager{docker: docker, interface_: iface}
}

func (m *Manager) execOrSidecar(ctx context.Context, containerID string, cmd []string) (string, error) {
	out, err := m.docker.Exec(ctx, containerID, cmd)
	if err == nil {
		return out, nil
	}
	// Target container might lack tc or NET_ADMIN capability. Fall back to sidecar container!
	sidecarOut, sidecarErr := m.docker.RunSidecar(ctx, containerID, cmd)
	if sidecarErr == nil {
		return sidecarOut, nil
	}
	return out, fmt.Errorf("in-container tc failed (%v), sidecar also failed (%v)", err, sidecarErr)
}

func (m *Manager) AddLatency(ctx context.Context, containerID string, delayMs, jitterMs int) (string, error) {
	cmd := []string{"tc", "qdisc", "add", "dev", m.interface_, "root", "netem",
		"delay", fmt.Sprintf("%dms", delayMs), fmt.Sprintf("%dms", jitterMs)}
	return m.execOrSidecar(ctx, containerID, cmd)
}

func (m *Manager) AddPacketLoss(ctx context.Context, containerID string, percent float64) (string, error) {
	cmd := []string{"tc", "qdisc", "add", "dev", m.interface_, "root", "netem",
		"loss", fmt.Sprintf("%.1f%%", percent)}
	return m.execOrSidecar(ctx, containerID, cmd)
}

func (m *Manager) AddCorruption(ctx context.Context, containerID string, percent float64) (string, error) {
	cmd := []string{"tc", "qdisc", "add", "dev", m.interface_, "root", "netem",
		"corrupt", fmt.Sprintf("%.1f%%", percent)}
	return m.execOrSidecar(ctx, containerID, cmd)
}

func (m *Manager) Clear(ctx context.Context, containerID string) (string, error) {
	cmd := []string{"tc", "qdisc", "del", "dev", m.interface_, "root"}
	out, err := m.execOrSidecar(ctx, containerID, cmd)
	if err != nil {
		errMsg := err.Error() + " " + out
		if strings.Contains(errMsg, "No such") || strings.Contains(errMsg, "Cannot find") || strings.Contains(errMsg, "Invalid argument") {
			return out, nil
		}
	}
	return out, err
}
