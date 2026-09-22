package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"pgvector/pgvector:0.8.6-pg18",
		postgres.WithInitScripts("../../local-development/init.sql"),
		postgres.WithDatabase("minimovie_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		panic(err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testPool.Close()
	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

func truncateAll(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tables := []string{
		"user_achievement", "watch_event", "watchlist_item",
		"movies", "series", "seasons", "episodes", "collections", "people",
		"auth_code", "sessions", "oauth_accounts",
		"provider_notifications_seen", "users",
	}
	for _, table := range tables {
		_, err := testPool.Exec(ctx, "DELETE FROM "+table)
		require.NoError(t, err)
	}
}

func createTestUser(t *testing.T) string {
	t.Helper()
	s := NewUserStore(testPool)
	result, err := s.UpsertFromOAuth(context.Background(), "google", "test-provider-id-"+t.Name(), "", nil, nil)
	require.NoError(t, err)
	return result.User.ID
}

// insertTestMovie writes a hydrated movies row and returns its id. A caller sets only the fields it
// asserts; a nil payload is replaced so the row counts as hydrated rather than as a skeleton.
func insertTestMovie(t *testing.T, m Movie) int {
	t.Helper()
	if m.Payload == nil {
		m.Payload = json.RawMessage(`{}`)
	}
	id, err := NewMovieStore(testPool).UpsertHydrated(context.Background(), testPool, m)
	require.NoError(t, err)
	return id
}

// insertTestSeries is insertTestMovie for series.
func insertTestSeries(t *testing.T, s Series) int {
	t.Helper()
	if s.Payload == nil {
		s.Payload = json.RawMessage(`{}`)
	}
	id, err := NewSeriesStore(testPool).UpsertHydrated(context.Background(), testPool, s)
	require.NoError(t, err)
	return id
}
