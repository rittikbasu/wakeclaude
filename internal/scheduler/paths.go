package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	appName = "WakeClaude"
)

type Store struct {
	BaseDir      string
	SchedulesDir string
	LogsDir      string
	Schedules    string
	Logs         string
}

func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	base := filepath.Join(home, "Library", "Application Support", appName)
	return &Store{
		BaseDir:      base,
		SchedulesDir: base,
		LogsDir:      filepath.Join(base, "logs"),
		Schedules:    filepath.Join(base, "schedules.json"),
		Logs:         filepath.Join(base, "logs.jsonl"),
	}, nil
}

func (s *Store) Ensure() error {
	if err := os.MkdirAll(s.SchedulesDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(s.LogsDir, 0o755); err != nil {
		return fmt.Errorf("create logs directory: %w", err)
	}
	return nil
}
