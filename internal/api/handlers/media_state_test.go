package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMediaState_MovieInWatchlistAndWatched(t *testing.T) {
	td := newTestHandlers(t)
	td.watchlistStore.checkItem = &store.WatchlistItem{ID: "wl-1", Status: "watched"}
	td.watchEventStore.events = []store.WatchEvent{{ID: "we-1", MediaType: "movie", MediaID: 550}}

	r := authedRequest(t, http.MethodGet, "/users/me/media-state?media_type=movie&media_id=550", nil)
	w := httptest.NewRecorder()

	td.handlers.GetMediaState(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, true, resp["inWatchlist"])
	assert.Equal(t, "wl-1", resp["watchlistItemId"])
	assert.Equal(t, true, resp["hasWatched"])
	assert.Equal(t, "we-1", resp["watchEventId"])
}

func TestGetMediaState_MovieNeitherInWatchlistNorWatched(t *testing.T) {
	td := newTestHandlers(t)
	// fakes default to nil/empty — nothing in watchlist, no events.

	r := authedRequest(t, http.MethodGet, "/users/me/media-state?media_type=movie&media_id=550", nil)
	w := httptest.NewRecorder()

	td.handlers.GetMediaState(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, false, resp["inWatchlist"])
	assert.Equal(t, false, resp["hasWatched"])
	_, hasItemID := resp["watchlistItemId"]
	_, hasEventID := resp["watchEventId"]
	assert.False(t, hasItemID, "watchlistItemId should be omitted when not in watchlist")
	assert.False(t, hasEventID, "watchEventId should be omitted when never watched")
}

// Episode/season views don't render the watchlist button — verify the handler
// skips the watchlist check for these media_types and only reports watch state.
// Episode and season watch state lives in /users/me/progress/{seriesId};
// /users/me/media-state only accepts movie and series.
func TestGetMediaState_RejectsEpisodeAndSeason(t *testing.T) {
	td := newTestHandlers(t)

	for _, mt := range []string{"episode", "season"} {
		r := authedRequest(t, http.MethodGet, "/users/me/media-state?media_type="+mt+"&media_id=9001", nil)
		w := httptest.NewRecorder()
		td.handlers.GetMediaState(w, r)
		require.Equal(t, http.StatusBadRequest, w.Code, "mediaType=%s should be rejected", mt)
	}
}

func TestGetMediaState_InvalidMediaType(t *testing.T) {
	td := newTestHandlers(t)

	r := authedRequest(t, http.MethodGet, "/users/me/media-state?media_type=bogus&media_id=1", nil)
	w := httptest.NewRecorder()

	td.handlers.GetMediaState(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetMediaState_MissingMediaID(t *testing.T) {
	td := newTestHandlers(t)

	r := authedRequest(t, http.MethodGet, "/users/me/media-state?media_type=movie", nil)
	w := httptest.NewRecorder()

	td.handlers.GetMediaState(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
