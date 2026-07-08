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

func (m *Manager) AddLatency(ctx context.Context, containerID string, delayMs, jitterMs int) (string, error) {
cmd := []string{"tc", "qdisc", "add", "dev", m.interface_, "root", "netem",
"delay", fmt.Sprintf("%dms", delayMs), fmt.Sprintf("%dms", jitterMs)}
return m.docker.Exec(ctx, containerID, cmd)
}

func (m *Manager) AddPacketLoss(ctx context.Context, containerID string, percent float64) (string, error) {
cmd := []string{"tc", "qdisc", "add", "dev", m.interface_, "root", "netem",
"loss", fmt.Sprintf("%.1f%%", percent)}
return m.docker.Exec(ctx, containerID, cmd)
}

func (m *Manager) AddCorruption(ctx context.Context, containerID string, percent float64) (string, error) {
cmd := []string{"tc", "qdisc", "add", "dev", m.interface_, "root", "netem",
"corrupt", fmt.Sprintf("%.1f%%", percent)}
return m.docker.Exec(ctx, containerID, cmd)
}

func (m *Manager) Clear(ctx context.Context, containerID string) (string, error) {
cmd := []string{"tc", "qdisc", "del", "dev", m.interface_, "root"}
out, err := m.docker.Exec(ctx, containerID, cmd)
if err != nil && strings.Contains(out, "No such") {
return out, nil
}
return out, err
}
