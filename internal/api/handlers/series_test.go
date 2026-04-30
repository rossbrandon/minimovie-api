package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSeries_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.series = &tmdb.Series{
		ID:              1399,
		Name:            "Breaking Bad",
		Overview:        "A high school chemistry teacher...",
		Genres:          []tmdb.Genre{{Name: "Drama"}},
		NumberOfSeasons: 5,
	}

	req := httptest.NewRequest(http.MethodGet, "/series/1399", nil)
	req = withChiParams(req, map[string]string{"id": "1399"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeries(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp SeriesDetails
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 1399, resp.ID)
	assert.Equal(t, "Breaking Bad", resp.Name)
	assert.Equal(t, 5, resp.NumberOfSeasons)
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
	td.mediaClient.seriesErr = tmdb.ErrNotFound

	req := httptest.NewRequest(http.MethodGet, "/series/999999", nil)
	req = withChiParams(req, map[string]string{"id": "999999"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeries(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
