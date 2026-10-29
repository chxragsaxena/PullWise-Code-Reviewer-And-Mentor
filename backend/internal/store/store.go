// Package store persists reviews and findings in PostgreSQL.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

// Store is the PostgreSQL implementation of the review persistence interfaces.
type Store struct {
	db *sql.DB
}

// New wraps an open database handle.
func New(db *sql.DB) *Store { return &Store{db: db} }

// Open connects to PostgreSQL and verifies the connection.
func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return db, nil
}

// SyncSkills makes the skills table mirror the loaded skills, for foreign keys.
func (s *Store) SyncSkills(ctx context.Context, loaded []skills.Skill) error {
	for _, sk := range loaded {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO skills (id, name, version, languages)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name, version = EXCLUDED.version, languages = EXCLUDED.languages`,
			sk.ID, sk.Name, sk.Version, sk.Languages)
		if err != nil {
			return fmt.Errorf("sync skill %q: %w", sk.ID, err)
		}
	}
	return nil
}

// parseID turns an API id into a database id; a failure means "not found".
func parseID(id string) (int64, bool) {
	n, err := strconv.ParseInt(id, 10, 64)
	return n, err == nil && n > 0
}

func nullableInt(n int) sql.NullInt32 {
	return sql.NullInt32{Int32: int32(n), Valid: n > 0}
}
