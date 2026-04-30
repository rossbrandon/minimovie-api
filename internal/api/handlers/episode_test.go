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

func TestGetEpisode_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.episode = &tmdb.EpisodeDetails{
		ID:            62085,
		Name:          "Pilot",
		EpisodeNumber: 1,
		SeasonNumber:  1,
		AirDate:       "2008-01-20",
		Runtime:       58,
		VoteAverage:   7.7,
	}

	req := httptest.NewRequest(http.MethodGet, "/series/1399/seasons/1/episodes/1", nil)
	req = withChiParams(req, map[string]string{
		"seriesId":      "1399",
		"seasonNumber":  "1",
		"episodeNumber": "1",
	})

	rr := httptest.NewRecorder()
	td.handlers.GetEpisode(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp EpisodeDetails
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 62085, resp.ID)
	assert.Equal(t, "Pilot", resp.Name)
	assert.Equal(t, 1, resp.EpisodeNumber)
	assert.Equal(t, 58, resp.Runtime)
}

func TestGetEpisode_InvalidSeriesID(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/series/abc/seasons/1/episodes/1", nil)
	req = withChiParams(req, map[string]string{
		"seriesId":      "abc",
		"seasonNumber":  "1",
		"episodeNumber": "1",
	})

	rr := httptest.NewRecorder()
	td.handlers.GetEpisode(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetEpisode_InvalidSeasonNumber(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/series/1399/seasons/abc/episodes/1", nil)
	req = withChiParams(req, map[string]string{
		"seriesId":      "1399",
		"seasonNumber":  "abc",
		"episodeNumber": "1",
	})

	rr := httptest.NewRecorder()
	td.handlers.GetEpisode(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetEpisode_InvalidEpisodeNumber(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/series/1399/seasons/1/episodes/abc", nil)
	req = withChiParams(req, map[string]string{
		"seriesId":      "1399",
		"seasonNumber":  "1",
		"episodeNumber": "abc",
	})

	rr := httptest.NewRecorder()
	td.handlers.GetEpisode(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
