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

func TestListWatchlist_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.watchlistStore.items = []store.WatchlistItem{
		{ID: "wl-1", MediaType: "movie", MediaID: 550, MediaTitle: "Fight Club", Status: "watched", AddedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "wl-2", MediaType: "series", MediaID: 1396, MediaTitle: "Breaking Bad", Status: "watching", AddedAt: time.Now(), UpdatedAt: time.Now()},
	}

	r := authedRequest(t, http.MethodGet, "/watchlist", nil)
	w := httptest.NewRecorder()

	td.handlers.ListWatchlist(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	items := resp["items"].([]any)
	assert.Len(t, items, 2)
	assert.Equal(t, float64(2), resp["total"])
}

func TestAddToWatchlist_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.watchlistStore.createdItem = &store.WatchlistItem{
		ID:        "wl-new",
		MediaType: "movie",
		MediaID:   550,
		Status:    "want_to_watch",
		AddedAt:   time.Now(),
		UpdatedAt: time.Now(),
	}

	body := strings.NewReader(`{"mediaType":"movie","mediaId":550,"status":"want_to_watch"}`)
	r := authedRequest(t, http.MethodPost, "/watchlist", body)
	w := httptest.NewRecorder()

	td.handlers.AddToWatchlist(w, r)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, "wl-new", resp["id"])
	assert.Equal(t, "movie", resp["mediaType"])
}

func TestAddToWatchlist_InvalidBody(t *testing.T) {
	td := newTestHandlers(t)

	body := strings.NewReader(`{bad json`)
	r := authedRequest(t, http.MethodPost, "/watchlist", body)
	w := httptest.NewRecorder()

	td.handlers.AddToWatchlist(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateWatchlistStatus_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.watchlistStore.updatedItem = &store.WatchlistItem{
		ID:        "wl-1",
		MediaType: "movie",
		MediaID:   550,
		Status:    "watched",
		AddedAt:   time.Now(),
		UpdatedAt: time.Now(),
	}

	body := strings.NewReader(`{"status":"watched"}`)
	r := authedRequest(t, http.MethodPatch, "/watchlist/wl-1", body)
	r = withChiParams(r, map[string]string{"id": "wl-1"})
	w := httptest.NewRecorder()

	td.handlers.UpdateWatchlistStatus(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, "watched", resp["status"])
}

func TestRemoveFromWatchlist_Success(t *testing.T) {
	td := newTestHandlers(t)

	r := authedRequest(t, http.MethodDelete, "/watchlist/wl-1", nil)
	r = withChiParams(r, map[string]string{"id": "wl-1"})
	w := httptest.NewRecorder()

	td.handlers.RemoveFromWatchlist(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestCheckWatchlist_Found(t *testing.T) {
	td := newTestHandlers(t)
	td.watchlistStore.checkItem = &store.WatchlistItem{
		ID:     "wl-1",
		Status: "watching",
	}

	r := authedRequest(t, http.MethodGet, "/watchlist/check?media_type=movie&media_id=550", nil)
	w := httptest.NewRecorder()

	td.handlers.CheckWatchlist(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, true, resp["exists"])
	item := resp["item"].(map[string]any)
	assert.Equal(t, "wl-1", item["id"])
	assert.Equal(t, "watching", item["status"])
}

func TestCheckWatchlist_NotFound(t *testing.T) {
	td := newTestHandlers(t)
	td.watchlistStore.checkItem = nil

	r := authedRequest(t, http.MethodGet, "/watchlist/check?media_type=movie&media_id=550", nil)
	w := httptest.NewRecorder()

	td.handlers.CheckWatchlist(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Equal(t, false, resp["exists"])
}
