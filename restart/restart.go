package restart

import "github.com/element-hq/chaos/config"

// Restarter is the interface to implement in order to restart
// a homserver.
type Restarter interface {
	Restart() error
	Config() *config.HomeserverConfig
}
