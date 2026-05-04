// Package storage provides the data access layer.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// User represents a user record in the database.
type User struct {
	ID             string
	Email          string
	Role           string
	DisplayName    string
	HashedPassword string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UserRepository provides CRUD access to the users table.
type UserRepository struct {
	store *DBStore
}

// NewUserRepository creates a UserRepository backed by the provided DBStore.
func NewUserRepository(db *DBStore) *UserRepository {
	return &UserRepository{store: db}
}

// FindByID retrieves a user by their unique ID.
// Returns sql.ErrNoRows if the user does not exist.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
	const q = `SELECT id, email, role, display_name, hashed_password, is_active, created_at, updated_at
	           FROM users WHERE id = $1`
	row := r.store.db.QueryRowContext(ctx, q, id)
	return scanUser(row)
}

// FindByEmail retrieves a user by their email address.
// Returns sql.ErrNoRows if no user has that email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	const q = `SELECT id, email, role, display_name, hashed_password, is_active, created_at, updated_at
	           FROM users WHERE email = $1`
	row := r.store.db.QueryRowContext(ctx, q, email)
	return scanUser(row)
}

// Save inserts a new user or updates an existing one (upsert by ID).
func (r *UserRepository) Save(ctx context.Context, u *User) error {
	const q = `INSERT INTO users (id, email, role, display_name, hashed_password, is_active, created_at, updated_at)
	           VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	           ON CONFLICT (id) DO UPDATE SET
	             email=$2, role=$3, display_name=$4, hashed_password=$5,
	             is_active=$6, updated_at=$8`
	_, err := r.store.db.ExecContext(ctx, q,
		u.ID, u.Email, u.Role, u.DisplayName, u.HashedPassword,
		u.IsActive, u.CreatedAt, time.Now())
	if err != nil {
		return fmt.Errorf("save user %s: %w", u.ID, err)
	}
	return nil
}

// Delete removes a user by ID. Returns nil if the user did not exist.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	_, err := r.store.db.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete user %s: %w", id, err)
	}
	return nil
}

// List returns paginated users ordered by created_at descending.
func (r *UserRepository) List(ctx context.Context, offset, limit int) ([]*User, error) {
	const q = `SELECT id, email, role, display_name, hashed_password, is_active, created_at, updated_at
	           FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.store.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (*User, error) {
	var u User
	err := s.Scan(&u.ID, &u.Email, &u.Role, &u.DisplayName,
		&u.HashedPassword, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}
