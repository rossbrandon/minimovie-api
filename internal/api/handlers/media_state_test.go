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
func TestGetMediaState_EpisodeSkipsWatchlistCheck(t *testing.T) {
	td := newTestHandlers(t)
	// If the handler were to call Check anyway, the fake would return this
	// non-nil item. The assertion below proves we never used it for episode.
	td.watchlistStore.checkItem = &store.WatchlistItem{ID: "should-not-appear"}
	td.watchEventStore.events = []store.WatchEvent{{ID: "ep-event", MediaType: "episode", MediaID: 9001}}

	r := authedRequest(t, http.MethodGet, "/users/me/media-state?media_type=episode&media_id=9001&series_id=1399", nil)
	w := httptest.NewRecorder()

	td.handlers.GetMediaState(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, false, resp["inWatchlist"], "episode views must not consult the watchlist")
	assert.Equal(t, true, resp["hasWatched"])
	assert.Equal(t, "ep-event", resp["watchEventId"])
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
