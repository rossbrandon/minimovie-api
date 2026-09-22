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

func TestGetMovie_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.catalog.movie = &catalog.Movie{
		Movie: &tmdb.Movie{
			ID:       550,
			Title:    "Fight Club",
			Overview: "An insomniac office worker...",
			Genres:   []tmdb.Genre{{Name: "Drama"}},
			Credits: tmdb.Credits{
				Cast: []tmdb.CastMember{{ID: 287, Name: "Brad Pitt"}, {ID: 819, Name: "Edward Norton"}},
			},
		},
		ID:     7,
		Slug:   "7-fight-club",
		People: catalog.PeopleDates{287: {ID: 3, DateOfBirth: "1963-12-18"}},
	}

	for _, id := range []string{"7", "7-fight-club"} {
		req := httptest.NewRequest(http.MethodGet, "/movies/"+id, nil)
		req = withChiParams(req, map[string]string{"id": id})

		rr := httptest.NewRecorder()
		td.handlers.GetMovie(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var resp MovieDetails
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
		assert.Equal(t, 7, resp.ID, "the response carries our id, not the provider's")
		assert.Equal(t, "7-fight-club", resp.Slug)
		assert.Equal(t, "Fight Club", resp.Title)
		assert.Equal(t, []string{"Drama"}, resp.Genres)
		require.Len(t, resp.Credits.Cast, 1, "a credited person without a row is left out")
		assert.Equal(t, 3, resp.Credits.Cast[0].ID)
		assert.Equal(t, "1963-12-18", resp.Credits.Cast[0].Birthday)
	}
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

	req := httptest.NewRequest(http.MethodGet, "/movies/999999-unknown", nil)
	req = withChiParams(req, map[string]string{"id": "999999-unknown"})

	rr := httptest.NewRecorder()
	td.handlers.GetMovie(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}
