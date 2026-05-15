package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type User struct {
	PhoneNumber  string
	LanguagePref string
	CreatedAt    time.Time
}

type UserRepository interface {
	GetUser(ctx context.Context, phone string) (*User, error)
	UpsertUser(ctx context.Context, phone, lang string) error
}

type PgUserRepository struct {
	db *sql.DB
}

func NewPgUserRepository(db *sql.DB) *PgUserRepository {
	return &PgUserRepository{db: db}
}

func (r *PgUserRepository) GetUser(ctx context.Context, phone string) (*User, error) {
	const q = `SELECT phone_number, language_pref, created_at FROM users WHERE phone_number = $1`
	var u User
	err := r.db.QueryRowContext(ctx, q, phone).Scan(&u.PhoneNumber, &u.LanguagePref, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

func (r *PgUserRepository) UpsertUser(ctx context.Context, phone, lang string) error {
	const q = `
		INSERT INTO users (phone_number, language_pref)
		VALUES ($1, $2)
		ON CONFLICT (phone_number) DO UPDATE SET language_pref = EXCLUDED.language_pref
	`
	if _, err := r.db.ExecContext(ctx, q, phone, lang); err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}
