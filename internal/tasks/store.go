package tasks

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore keeps the triggers and last runs in the tables task_triggers
// and task_runs of the server's database.
type DBStore struct {
	db *pgxpool.Pool
}

// NewStore returns the store of db.
func NewStore(db *pgxpool.Pool) *DBStore {
	return &DBStore{db: db}
}

// LoadTriggers gives the triggers saved, by task key.
func (s *DBStore) LoadTriggers(ctx context.Context) (map[string][]Trigger, error) {
	rows, err := s.db.Query(ctx, "SELECT task_key, triggers FROM task_triggers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]Trigger{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		triggers := []Trigger{}
		if err := json.Unmarshal(raw, &triggers); err != nil {
			return nil, err
		}
		result[key] = triggers
	}
	return result, rows.Err()
}

// SaveTriggers replaces the triggers of the task of key.
func (s *DBStore) SaveTriggers(ctx context.Context, key string, triggers []Trigger) error {
	raw, err := json.Marshal(triggers)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO task_triggers (task_key, triggers, changed_at) VALUES ($1, $2, now())
		ON CONFLICT (task_key) DO UPDATE SET triggers = excluded.triggers, changed_at = excluded.changed_at`, key, raw)
	return err
}

// DeleteTriggers returns the task of key to its default triggers.
func (s *DBStore) DeleteTriggers(ctx context.Context, key string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM task_triggers WHERE task_key = $1", key)
	return err
}

// LoadRuns gives the last run of each task, by key.
func (s *DBStore) LoadRuns(ctx context.Context) (map[string]Result, error) {
	rows, err := s.db.Query(ctx, "SELECT task_key, started_at, ended_at, status, error FROM task_runs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]Result{}
	for rows.Next() {
		var key string
		var run Result
		var status string
		if err := rows.Scan(&key, &run.Start, &run.End, &status, &run.Error); err != nil {
			return nil, err
		}
		run.Status = Status(status)
		run.Start, run.End = run.Start.In(time.Local), run.End.In(time.Local)
		result[key] = run
	}
	return result, rows.Err()
}

// SaveRun keeps result as the last run of the task of key.
func (s *DBStore) SaveRun(ctx context.Context, key string, result Result) error {
	_, err := s.db.Exec(ctx, `INSERT INTO task_runs (task_key, started_at, ended_at, status, error) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (task_key) DO UPDATE SET started_at = excluded.started_at, ended_at = excluded.ended_at,
			status = excluded.status, error = excluded.error`, key, result.Start, result.End, string(result.Status), result.Error)
	return err
}
