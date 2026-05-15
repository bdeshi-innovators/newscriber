//go:build integration

package users_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	appdb "voicescribe-webhook/internal/db"
	"voicescribe-webhook/internal/users"
)

func startPostgres(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("voicescribe"),
		tcpostgres.WithUsername("voicescribe"),
		tcpostgres.WithPassword("voicescribe"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := appdb.Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

func TestPgUserRepository_UpsertAndGet(t *testing.T) {
	conn := startPostgres(t)
	repo := users.NewPgUserRepository(conn)
	ctx := context.Background()

	got, err := repo.GetUser(ctx, "+391112223333")
	if err != nil {
		t.Fatalf("get miss: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil user, got %+v", got)
	}

	if err := repo.UpsertUser(ctx, "+391112223333", "en"); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}

	got, err = repo.GetUser(ctx, "+391112223333")
	if err != nil {
		t.Fatalf("get hit: %v", err)
	}
	if got == nil || got.LanguagePref != "en" {
		t.Fatalf("after insert: got %+v want lang=en", got)
	}

	if err := repo.UpsertUser(ctx, "+391112223333", "bn"); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got, err = repo.GetUser(ctx, "+391112223333")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.LanguagePref != "bn" {
		t.Fatalf("after update: got %q want bn", got.LanguagePref)
	}
}
