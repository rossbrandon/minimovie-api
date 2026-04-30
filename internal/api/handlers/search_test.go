package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.searchResults = &tmdb.SearchResults{
		Page:         1,
		TotalPages:   1,
		TotalResults: 1,
		Results: []tmdb.SearchResult{
			{
				ID:        550,
				MediaType: tmdb.MediaTypeMovie,
				SearchResultMovie: tmdb.SearchResultMovie{
					Title:       "Fight Club",
					ReleaseDate: "1999-10-15",
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/search?q=fight+club", nil)
	rr := httptest.NewRecorder()
	td.handlers.Search(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp SearchResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 1, resp.TotalResults)
	assert.Equal(t, "Fight Club", resp.Results[0].Title)
	assert.Equal(t, MediaTypeMovie, resp.Results[0].MediaType)
}

func TestSearch_MissingQuery(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rr := httptest.NewRecorder()
	td.handlers.Search(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSearch_InvalidType(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/search?q=test&type=invalid", nil)
	rr := httptest.NewRecorder()
	td.handlers.Search(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSearch_ClientError(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.searchErr = errors.New("network failure")

	req := httptest.NewRequest(http.MethodGet, "/search?q=test", nil)
	rr := httptest.NewRecorder()
	td.handlers.Search(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
