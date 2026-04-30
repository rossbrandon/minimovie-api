package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStats_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.statsStore.stats = &store.StatsResult{
		TotalMoviesWatched:      15,
		TotalSeriesCompleted:    3,
		TotalEpisodesWatched:    42,
		EstimatedMinutesWatched: 4800,
		GenreBreakdown:          []store.GenreStat{{Genre: "Drama", Count: 10}},
		MonthlyActivity:         []store.MonthActivity{{Month: "2026-04", MoviesWatched: 5, EpisodesWatched: 12}},
		CurrentStreak:           3,
		LongestStreak:           7,
	}

	r := authedRequest(t, http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()

	td.handlers.GetStats(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, float64(15), resp["totalMoviesWatched"])
	assert.Equal(t, float64(3), resp["totalSeriesCompleted"])
	assert.Equal(t, float64(42), resp["totalEpisodesWatched"])
	assert.Equal(t, float64(4800), resp["estimatedMinutesWatched"])
	assert.Equal(t, float64(3), resp["currentStreak"])
	assert.Equal(t, float64(7), resp["longestStreak"])
}
