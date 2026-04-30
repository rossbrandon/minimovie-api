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

func TestGetMovie_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.movie = &tmdb.Movie{
		ID:       550,
		Title:    "Fight Club",
		Overview: "An insomniac office worker...",
		Genres:   []tmdb.Genre{{Name: "Drama"}},
	}

	req := httptest.NewRequest(http.MethodGet, "/movies/550", nil)
	req = withChiParams(req, map[string]string{"id": "550"})

	rr := httptest.NewRecorder()
	td.handlers.GetMovie(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp MovieDetails
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 550, resp.ID)
	assert.Equal(t, "Fight Club", resp.Title)
	assert.Equal(t, []string{"Drama"}, resp.Genres)
}

func TestGetMovie_InvalidID(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/movies/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})

	rr := httptest.NewRecorder()
	td.handlers.GetMovie(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetMovie_NotFound(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.movieErr = tmdb.ErrNotFound

	req := httptest.NewRequest(http.MethodGet, "/movies/999999", nil)
	req = withChiParams(req, map[string]string{"id": "999999"})

	rr := httptest.NewRecorder()
	td.handlers.GetMovie(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}
