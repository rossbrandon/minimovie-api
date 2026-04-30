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

func TestGetSeason_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.season = &tmdb.SeasonDetails{
		ID:           3572,
		Name:         "Season 1",
		SeasonNumber: 1,
		AirDate:      "2008-01-20",
		Episodes: []tmdb.Episode{
			{
				ID:            62085,
				Name:          "Pilot",
				EpisodeNumber: 1,
				SeasonNumber:  1,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/series/1399/seasons/1", nil)
	req = withChiParams(req, map[string]string{"seriesId": "1399", "seasonNumber": "1"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeason(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp SeasonDetails
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 3572, resp.ID)
	assert.Equal(t, "Season 1", resp.Name)
	require.Len(t, resp.Episodes, 1)
	assert.Equal(t, "Pilot", resp.Episodes[0].Name)
}

func TestGetSeason_InvalidSeriesID(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/series/abc/seasons/1", nil)
	req = withChiParams(req, map[string]string{"seriesId": "abc", "seasonNumber": "1"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeason(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetSeason_InvalidSeasonNumber(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/series/1399/seasons/abc", nil)
	req = withChiParams(req, map[string]string{"seriesId": "1399", "seasonNumber": "abc"})

	rr := httptest.NewRecorder()
	td.handlers.GetSeason(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
