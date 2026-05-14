package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWatchEvent_Movie(t *testing.T) {
	td := newTestHandlers(t)
	td.watchEventStore.createdEvent = &store.WatchEvent{
		ID:        "ev-1",
		MediaType: "movie",
		MediaID:   550,
	}

	body := strings.NewReader(`{"mediaType":"movie","mediaId":550,"justWatched":true,"timezone":"America/Chicago"}`)
	r := authedRequest(t, http.MethodPost, "/watch-events", body)
	w := httptest.NewRecorder()

	td.handlers.CreateWatchEvent(w, r)

	require.Equal(t, http.StatusAccepted, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, "accepted", resp["status"])
}

func TestCreateWatchEvent_InvalidTimezone(t *testing.T) {
	td := newTestHandlers(t)

	body := strings.NewReader(`{"mediaType":"movie","mediaId":550,"timezone":"Not/A/Zone"}`)
	r := authedRequest(t, http.MethodPost, "/watch-events", body)
	w := httptest.NewRecorder()

	td.handlers.CreateWatchEvent(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateWatchEvent_MissingFields(t *testing.T) {
	td := newTestHandlers(t)

	body := strings.NewReader(`{"mediaType":"movie","mediaId":0}`)
	r := authedRequest(t, http.MethodPost, "/watch-events", body)
	w := httptest.NewRecorder()

	td.handlers.CreateWatchEvent(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Series-level watch events have been retired in favor of per-season /
// per-episode marks. Defensive against stale clients still posting them.
func TestCreateWatchEvent_RejectsSeries(t *testing.T) {
	td := newTestHandlers(t)

	body := strings.NewReader(`{"mediaType":"series","mediaId":1399,"timezone":"UTC"}`)
	r := authedRequest(t, http.MethodPost, "/watch-events", body)
	w := httptest.NewRecorder()

	td.handlers.CreateWatchEvent(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListWatchEvents_Success(t *testing.T) {
	td := newTestHandlers(t)
	now := time.Now()
	td.watchEventStore.events = []store.WatchEvent{
		{ID: "ev-1", MediaType: "movie", MediaID: 550, MediaTitle: "Fight Club", Timezone: "UTC", Genres: []string{"Drama"}, CreatedAt: now},
		{ID: "ev-2", MediaType: "movie", MediaID: 680, MediaTitle: "Pulp Fiction", Timezone: "UTC", Genres: []string{"Action", "Crime"}, CreatedAt: now},
	}

	r := authedRequest(t, http.MethodGet, "/watch-events", nil)
	w := httptest.NewRecorder()

	td.handlers.ListWatchEvents(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	events := resp["events"].([]any)
	assert.Len(t, events, 2)
	assert.Equal(t, float64(2), resp["total"])
}
