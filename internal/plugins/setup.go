package plugins

import (
	"gremlin-in-a-box/internal/dockerapi"
	"gremlin-in-a-box/internal/network"
)

func RegisterDefaults(docker *dockerapi.Client) {
	net := network.NewManager(docker, "eth0")

	Register(NewLatencyPlugin(net))
	Register(NewPacketLossPlugin(net))
	Register(NewCorruptionPlugin(net))
	Register(NewContainerKillPlugin(docker))
	Register(NewContainerPausePlugin(docker))
	Register(NewCPUPlugin(docker))
}
