// Package storage provides the data access layer.
package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// DBStore wraps a *sql.DB with application-level helpers.
type DBStore struct {
	db      *sql.DB
	dsn     string
	timeout time.Duration
}

// Connect opens a database connection using the provided DSN.
// It verifies the connection with a ping before returning.
func Connect(dsn string) (*DBStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	store := &DBStore{
		db:      db,
		dsn:     dsn,
		timeout: 5 * time.Second,
	}
	if err := store.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return store, nil
}

// Ping verifies the database connection is alive.
func (s *DBStore) Ping() error {
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	return nil
}

// Close releases all database connections.
func (s *DBStore) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced queries.
func (s *DBStore) DB() *sql.DB {
	return s.db
}
