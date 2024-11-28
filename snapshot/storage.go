package snapshot

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Storage struct {
	db *sqlx.DB
}

func NewStorage(filePath string) (*Storage, error) {
	db, err := sqlx.Open("sqlite3", filePath)
	if err != nil {
		return nil, err
	}
	db.Exec(`
	CREATE TABLE IF NOT EXISTS snapshots(
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		homeserver TEXT NOT NULL,
		process TEXT NOT NULL,
		memory_bytes BIGINT NOT NULL,
		cpu_millis BIGINT NOT NULL
	);`)
	return &Storage{
		db: db,
	}, nil
}

func (s *Storage) WriteSnapshot(snap Snapshot) error {
	if len(snap.ProcessEntries) == 0 {
		return nil
	}
	_, err := s.db.NamedExec(
		`INSERT INTO snapshots(homeserver,process,memory_bytes,cpu_millis) VALUES(:homeserver, :process, :memory_bytes, :cpu_millis)`,
		snap.ProcessEntries,
	)
	if err != nil {
		return fmt.Errorf("WriteSnapshot: %s", err)
	}
	return nil
}
