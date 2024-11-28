package snapshot

import (
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Snapshot struct {
	ProcessEntries []ProcessSnapshot
}

func (s Snapshot) String() string {
	stats := make([]string, len(s.ProcessEntries))
	for i, p := range s.ProcessEntries {
		stats[i] = p.String()
	}
	return strings.Join(stats, "\n")
}

type ProcessSnapshot struct {
	Homeserver  string `db:"homeserver"`
	ProcessName string `db:"process"`
	MilliCPUs   int64  `db:"cpu_millis"`
	MemoryBytes int64  `db:"memory_bytes"`
}

func (ps ProcessSnapshot) String() string {
	return fmt.Sprintf("%s (%s) CPU=%vm Mem=%dMB", ps.Homeserver, ps.ProcessName, ps.MilliCPUs, (ps.MemoryBytes/1024)/1024)
}

// Snapshotter is the interface required to perform metric snapshots on homeservers
type Snapshotter interface {
	Snapshot() (*Snapshot, error)
}
