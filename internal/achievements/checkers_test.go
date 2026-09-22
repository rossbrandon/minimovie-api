package achievements

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
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
	s := store.NewUserStore(testPool)
	result, err := s.UpsertFromOAuth(context.Background(), "google", "test-provider-id-"+t.Name(), "", nil, nil)
	require.NoError(t, err)
	return result.User.ID
}

// insertMovie writes a hydrated movies row, which is where the watchlist reads a title's rating and
// release year from, and returns its id.
func insertMovie(t *testing.T, m store.Movie) int {
	t.Helper()
	m.Payload = json.RawMessage(`{}`)
	id, err := store.NewMovieStore(testPool).UpsertHydrated(context.Background(), testPool, m)
	require.NoError(t, err)
	return id
}

func insertMovieFromYear(t *testing.T, sourceID, year int) int {
	t.Helper()
	release := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	return insertMovie(t, store.Movie{
		SourceID:    sourceID,
		Title:       fmt.Sprintf("Movie from %d", year),
		ReleaseDate: &release,
	})
}

func TestCheckCenturyClub(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := store.NewWatchlistStore(testPool)
	we := store.NewWatchEventStore(testPool)

	for i := 1; i <= 99; i++ {
		_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", i, "watched", fmt.Sprintf("Movie %d", i))
		require.NoError(t, err)
	}

	earned, _, _, _ := checkCenturyClub(ctx, userID, wl, we)
	assert.False(t, earned, "99 movies should not earn century club")

	_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", 100, "watched", "Movie 100")
	require.NoError(t, err)

	earned, mediaType, _, _ := checkCenturyClub(ctx, userID, wl, we)
	assert.True(t, earned, "100 movies should earn century club")
	assert.Equal(t, "movie", mediaType)
}

func TestCheckGenreExplorer(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := store.NewWatchlistStore(testPool)
	we := store.NewWatchEventStore(testPool)

	createWatchEvent := func(mediaID int, genres []string) {
		t.Helper()
		_, err := we.Create(ctx, store.WatchEventCreate{
			UserID:    userID,
			MediaType: "movie",
			MediaID:   mediaID,
			Timezone:  "UTC",
			Meta: store.ResolvedMedia{
				Title:  fmt.Sprintf("Genre Movie %d", mediaID),
				Genres: genres,
			},
		})
		require.NoError(t, err)
	}

	genreNames := []string{"Action", "Comedy", "Drama", "Horror", "Romance",
		"Thriller", "Sci-Fi", "Fantasy", "Documentary", "Animation",
		"Adventure", "Mystery", "Crime", "Western", "Musical"}

	for i := 0; i < 4; i++ {
		createWatchEvent(i+1, []string{genreNames[i]})
	}

	earned, _, _, _ := checkGenreExplorerBronze(ctx, userID, wl, we)
	assert.False(t, earned, "4 genres should not earn bronze")

	createWatchEvent(5, []string{genreNames[4]})

	earned, mediaType, _, _ := checkGenreExplorerBronze(ctx, userID, wl, we)
	assert.True(t, earned, "5 genres should earn bronze")
	assert.Equal(t, "movie", mediaType)

	for i := 5; i < 10; i++ {
		createWatchEvent(i+1, []string{genreNames[i]})
	}

	earned, mediaType, _, _ = checkGenreExplorerSilver(ctx, userID, wl, we)
	assert.True(t, earned, "10 genres should earn silver")
	assert.Equal(t, "movie", mediaType)

	for i := 10; i < 15; i++ {
		createWatchEvent(i+1, []string{genreNames[i]})
	}

	earned, mediaType, _, _ = checkGenreExplorerGold(ctx, userID, wl, we)
	assert.True(t, earned, "15 genres should earn gold")
	assert.Equal(t, "movie", mediaType)
}

func TestCheckCriticsPick(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := store.NewWatchlistStore(testPool)
	we := store.NewWatchEventStore(testPool)

	vote := 8.5
	for i := 1; i <= 9; i++ {
		movieID := insertMovie(t, store.Movie{
			SourceID:    i,
			Title:       fmt.Sprintf("Great Movie %d", i),
			VoteAverage: &vote,
		})
		_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", movieID, "watched", "")
		require.NoError(t, err)
	}

	earned, _, _, _ := checkCriticsPick(ctx, userID, wl, we)
	assert.False(t, earned, "9 high-rated movies should not earn critics pick")

	movieID := insertMovie(t, store.Movie{
		SourceID:    10,
		Title:       "Great Movie 10",
		VoteAverage: &vote,
	})
	_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", movieID, "watched", "")
	require.NoError(t, err)

	earned, mediaType, _, _ := checkCriticsPick(ctx, userID, wl, we)
	assert.True(t, earned, "10 high-rated movies should earn critics pick")
	assert.Equal(t, "movie", mediaType)
}

func TestCheckTimeTraveler(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := store.NewWatchlistStore(testPool)
	we := store.NewWatchEventStore(testPool)

	decades := []int{1970, 1985, 1993, 2004}
	for i, year := range decades {
		_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", insertMovieFromYear(t, i+1, year), "watched", "")
		require.NoError(t, err)
	}

	earned, _, _, _ := checkTimeTraveler(ctx, userID, wl, we)
	assert.False(t, earned, "4 decades should not earn time traveler")

	_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", insertMovieFromYear(t, 5, 2015), "watched", "")
	require.NoError(t, err)

	earned, mediaType, _, _ := checkTimeTraveler(ctx, userID, wl, we)
	assert.True(t, earned, "5 decades should earn time traveler")
	assert.Equal(t, "movie", mediaType)
}

func TestCheckMarathonRunner(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := store.NewWatchlistStore(testPool)
	we := store.NewWatchEventStore(testPool)

	tz := "America/Chicago"
	loc, _ := time.LoadLocation(tz)
	baseTime := time.Date(2025, 6, 15, 10, 0, 0, 0, loc)
	seriesID := 42

	createEpisode := func(mediaID, ep int, watchedAt time.Time) {
		t.Helper()
		sn := 1
		wa := watchedAt
		_, err := we.Create(ctx, store.WatchEventCreate{
			UserID:        userID,
			MediaType:     "episode",
			MediaID:       mediaID,
			SeriesID:      &seriesID,
			SeasonNumber:  &sn,
			EpisodeNumber: &ep,
			WatchedAt:     &wa,
			Timezone:      tz,
			Meta: store.ResolvedMedia{
				Title:  fmt.Sprintf("Episode %d", ep),
				Genres: []string{},
			},
		})
		require.NoError(t, err)
	}

	for i := 1; i <= 4; i++ {
		createEpisode(i, i, baseTime.Add(time.Duration(i)*time.Hour))
	}

	earned, _, _, _ := checkMarathonRunner(ctx, userID, wl, we)
	assert.False(t, earned, "4 episodes in a day should not earn marathon runner")

	createEpisode(5, 5, baseTime.Add(5*time.Hour))

	earned, mediaType, mediaID, _ := checkMarathonRunner(ctx, userID, wl, we)
	assert.True(t, earned, "5 episodes in a day should earn marathon runner")
	assert.Equal(t, "series", mediaType)
	assert.Equal(t, seriesID, mediaID)
}

func TestCheckNightOwl(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := store.NewWatchlistStore(testPool)
	we := store.NewWatchEventStore(testPool)

	tz := "America/New_York"
	loc, _ := time.LoadLocation(tz)

	t.Run("2am qualifies", func(t *testing.T) {
		wa := time.Date(2025, 6, 15, 2, 0, 0, 0, loc)
		_, err := we.Create(ctx, store.WatchEventCreate{
			UserID:    userID,
			MediaType: "movie",
			MediaID:   1,
			WatchedAt: &wa,
			Timezone:  tz,
			Meta:      store.ResolvedMedia{Title: "Night Movie", Genres: []string{}},
		})
		require.NoError(t, err)

		earned, mediaType, mediaID, _ := checkNightOwl(ctx, userID, wl, we)
		assert.True(t, earned)
		assert.Equal(t, "movie", mediaType)
		assert.Equal(t, 1, mediaID)
	})

	t.Run("10am does not qualify", func(t *testing.T) {
		truncateAll(t)
		userID := createTestUser(t)

		wa := time.Date(2025, 6, 15, 10, 0, 0, 0, loc)
		_, err := we.Create(ctx, store.WatchEventCreate{
			UserID:    userID,
			MediaType: "movie",
			MediaID:   2,
			WatchedAt: &wa,
			Timezone:  tz,
			Meta:      store.ResolvedMedia{Title: "Morning Movie", Genres: []string{}},
		})
		require.NoError(t, err)

		earned, _, _, _ := checkNightOwl(ctx, userID, wl, we)
		assert.False(t, earned)
	})
}

func TestCheckEarlyBird(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := store.NewWatchlistStore(testPool)
	we := store.NewWatchEventStore(testPool)

	tz := "America/New_York"
	loc, _ := time.LoadLocation(tz)

	t.Run("6am qualifies", func(t *testing.T) {
		wa := time.Date(2025, 6, 15, 6, 0, 0, 0, loc)
		_, err := we.Create(ctx, store.WatchEventCreate{
			UserID:    userID,
			MediaType: "movie",
			MediaID:   1,
			WatchedAt: &wa,
			Timezone:  tz,
			Meta:      store.ResolvedMedia{Title: "Early Movie", Genres: []string{}},
		})
		require.NoError(t, err)

		earned, mediaType, mediaID, _ := checkEarlyBird(ctx, userID, wl, we)
		assert.True(t, earned)
		assert.Equal(t, "movie", mediaType)
		assert.Equal(t, 1, mediaID)
	})

	t.Run("8am does not qualify", func(t *testing.T) {
		truncateAll(t)
		userID := createTestUser(t)

		wa := time.Date(2025, 6, 15, 8, 0, 0, 0, loc)
		_, err := we.Create(ctx, store.WatchEventCreate{
			UserID:    userID,
			MediaType: "movie",
			MediaID:   2,
			WatchedAt: &wa,
			Timezone:  tz,
			Meta:      store.ResolvedMedia{Title: "Late Morning Movie", Genres: []string{}},
		})
		require.NoError(t, err)

		earned, _, _, _ := checkEarlyBird(ctx, userID, wl, we)
		assert.False(t, earned)
	})
}
