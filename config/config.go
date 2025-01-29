package config

import (
	"fmt"
	"os"

	yaml "gopkg.in/yaml.v3"
)

type Chaos struct {
	Verbose   bool `yaml:"verbose"`
	WSPort    int  `yaml:"ws_port"`
	MITMProxy struct {
		ContainerURL string `yaml:"container_url"`
		HostDomain   string `yaml:"host_domain"`
	} `yaml:"mitm_proxy"`
	Homeservers []HomeserverConfig `yaml:"homeservers"`
	Test        TestConfig         `yaml:"test"`
}

type TestConfig struct {
	Seed                   int64  `yaml:"seed"`
	NumUsers               int    `yaml:"num_users"`
	NumRooms               int    `yaml:"num_rooms"`
	OpsPerTick             int    `yaml:"ops_per_tick"`
	RoomVersion            string `yaml:"room_version"`
	SendToLeaveProbability int    `yaml:"send_to_leave_probability"`
	Netsplits              struct {
		DurationSecs int `yaml:"duration_secs"`
		FreeSecs     int `yaml:"free_secs"`
	} `yaml:"netsplits"`
	Restarts struct {
		IntervalSecs int      `yaml:"interval_secs"`
		RoundRobin   []string `yaml:"round_robin"`
	} `yaml:"restarts"`
	Convergence struct {
		Enabled            bool `yaml:"enabled"`
		CheckEveryNTicks   int  `yaml:"check_every_n_ticks"`
		BufferDurationSecs int  `yaml:"buffer_secs"`
	}
	SnapshotDB string `yaml:"snapshot_db"` // path to sqlite3 file to write snapshot data to
}

type HomeserverConfig struct {
	BaseURL  string `yaml:"url"`
	Domain   string `yaml:"domain"`
	Snapshot struct {
		Type string         `yaml:"type"`
		Data map[string]any `yaml:"data"` // custom data for the snapshot type TODO: s/data/config/
	} `yaml:"snapshot"`
	Restart struct {
		Type   string         `yaml:"type"`
		Config map[string]any `yaml:"config"` // custom config for the restart type
	} `yaml:"restart"`
}

func OpenFile(cfgPath string) (*Chaos, error) {
	input, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("ReadFile %s: %s", cfgPath, err)
	}
	var cfg Chaos
	if err := yaml.Unmarshal(input, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %s", err)
	}
	return &cfg, nil
}

// UnmarshalInto round trips the provided config into the typed config type provided.
// This can be used to convert map[string]any into a typed configuration.
func UnmarshalInto[T any](config map[string]any) (T, error) {
	var typedConfig T
	b, err := yaml.Marshal(config)
	if err != nil {
		return typedConfig, fmt.Errorf("invalid config: %s", err)
	}
	return typedConfig, yaml.Unmarshal(b, &typedConfig)
}
