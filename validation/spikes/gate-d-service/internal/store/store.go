package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS service_boots (
        id INTEGER PRIMARY KEY CHECK (id = 1),
        count INTEGER NOT NULL
    )`); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) RecordBoot(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin boot transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO service_boots(id, count) VALUES(1, 1)
        ON CONFLICT(id) DO UPDATE SET count=count+1`); err != nil {
		return 0, fmt.Errorf("record service boot: %w", err)
	}
	var count int64
	if err := tx.QueryRowContext(ctx, "SELECT count FROM service_boots WHERE id=1").Scan(&count); err != nil {
		return 0, fmt.Errorf("read service boot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit service boot: %w", err)
	}
	return count, nil
}

func (s *Store) Close() error { return s.db.Close() }
