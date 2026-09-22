package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSeries_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.catalog.series = &catalog.Series{
		Series: &tmdb.Series{
			ID:              1399,
			Name:            "Breaking Bad",
			Overview:        "A high school chemistry teacher...",
			Genres:          []tmdb.Genre{{Name: "Drama"}},
			NumberOfSeasons: 5,
			Seasons:         []tmdb.Season{{ID: 3572, SeasonNumber: 1}, {ID: 3573, SeasonNumber: 2}},
		},
		ID:        9,
		Slug:      "9-breaking-bad",
		SeasonIDs: catalog.IDs{1: 11},
	}

	req := httptest.NewRequest(http.MethodGet, "/series/9-breaking-bad", nil)
	req = withChiParams(req, map[string]string{"id": "9-breaking-bad"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeries(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp SeriesDetails
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 9, resp.ID)
	assert.Equal(t, "9-breaking-bad", resp.Slug)
	assert.Equal(t, "Breaking Bad", resp.Name)
	assert.Equal(t, 5, resp.NumberOfSeasons)
	require.Len(t, resp.Seasons, 1, "a season without a row is left out")
	assert.Equal(t, 11, resp.Seasons[0].ID)
}

func TestGetSeries_InvalidID(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/series/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeries(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetSeries_NotFound(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/series/999999", nil)
	req = withChiParams(req, map[string]string{"id": "999999"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeries(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
