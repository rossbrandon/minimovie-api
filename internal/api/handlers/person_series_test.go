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

func TestGetPersonSeriesCredits_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.catalog.series = &catalog.Series{
		Series: &tmdb.Series{
			ID: 1396, Name: "Breaking Bad", NumberOfSeasons: 2,
			AggregateCredits: tmdb.AggregateCredits{Cast: []tmdb.AggregateCastMember{
				{ID: 17419, Name: "Bryan Cranston", TotalEpisodeCount: 3, Roles: []tmdb.Role{{Character: "Walter"}}},
			}},
		},
		ID:     9,
		People: catalog.PeopleDates{17419: {ID: 5}},
	}
	td.catalog.seasons = map[int]*tmdb.SeasonDetails{
		1: {
			SeasonNumber: 1, Name: "Season 1",
			Episodes:         []tmdb.Episode{{EpisodeNumber: 1, Name: "Pilot"}, {EpisodeNumber: 2, Name: "Cat"}},
			AggregateCredits: tmdb.AggregateCredits{Cast: []tmdb.AggregateCastMember{{ID: 17419, TotalEpisodeCount: 2}}},
		},
		2: {
			SeasonNumber: 2, Name: "Season 2",
			Episodes: []tmdb.Episode{
				{EpisodeNumber: 1, Name: "Seven", GuestStars: []tmdb.CastMember{{ID: 17419}}},
				{EpisodeNumber: 2, Name: "Grilled"},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/series/9/person/5/credits", nil)
	req = withChiParams(req, map[string]string{"seriesId": "9", "personId": "5"})
	rr := httptest.NewRecorder()
	td.handlers.GetPersonSeriesCredits(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp PersonSeriesCredits
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 5, resp.Person.ID, "our ids, not the provider's")
	assert.Equal(t, 9, resp.Series.ID)
	assert.Equal(t, 3, resp.TotalEpisodeCount)
	assert.Equal(t, []RoleSummary{{Character: "Walter"}}, resp.Roles)
	require.Len(t, resp.Seasons, 2)
	assert.Len(t, resp.Seasons[0].Episodes, 2, "in every episode of season 1")
	require.Len(t, resp.Seasons[1].Episodes, 1, "a guest in one episode of season 2")
	assert.Equal(t, "Seven", resp.Seasons[1].Episodes[0].Name)
}

func TestGetPersonSeriesCredits_PersonNotInCredits(t *testing.T) {
	td := newTestHandlers(t)
	td.catalog.series = &catalog.Series{Series: &tmdb.Series{ID: 1396}, ID: 9}

	req := httptest.NewRequest(http.MethodGet, "/series/9/person/5/credits", nil)
	req = withChiParams(req, map[string]string{"seriesId": "9", "personId": "5"})
	rr := httptest.NewRecorder()
	td.handlers.GetPersonSeriesCredits(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
