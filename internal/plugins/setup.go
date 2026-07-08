package plugins

import (
"gremlin-in-a-box/internal/dockerapi"
"gremlin-in-a-box/internal/network"
)

func RegisterDefaults(docker *dockerapi.Client) {
net := network.NewManager(docker, "eth0")

Register(NewLatencyPlugin(net))
Register(NewContainerKillPlugin(docker))
Register(NewCPUPlugin(docker))
}
