package restart

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/element-hq/chaos/config"
)

const RestartTypeDocker = "docker"

type DockerConfig struct {
	TimeoutSecs   *int   `yaml:"timeout_secs"`
	Signal        string `yaml:"signal"`
	ContainerName string `yaml:"container_name"`
}

type Docker struct {
	apiClient     *client.Client
	hsConfig      *config.HomeserverConfig
	containerName string
	timeoutSecs   int
	signal        string
}

func NewDockerRestarter(hsc config.HomeserverConfig) (Restarter, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client from env: %s", err)
	}
	restartConfig, err := config.UnmarshalInto[DockerConfig](hsc.Restart.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}
	signal := "SIGTERM"
	if restartConfig.Signal != "" {
		signal = restartConfig.Signal
	}
	timeoutSecs := 3
	if restartConfig.TimeoutSecs != nil {
		timeoutSecs = *restartConfig.TimeoutSecs
	}
	return &Docker{
		apiClient:     apiClient,
		containerName: hsc.Restart.Config["container_name"].(string),
		hsConfig:      &hsc,
		timeoutSecs:   timeoutSecs,
		signal:        signal,
	}, nil
}

func (d *Docker) Config() *config.HomeserverConfig {
	return d.hsConfig
}

func (d *Docker) Restart() error {
	return d.apiClient.ContainerRestart(context.Background(), d.containerName, container.StopOptions{
		Timeout: &d.timeoutSecs,
		Signal:  d.signal,
	})
}
