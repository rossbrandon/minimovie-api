package handlers

import (
	"net/http"
	"strconv"

	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
)

type mediaStateResponse struct {
	InWatchlist     bool    `json:"inWatchlist"`
	WatchlistItemID *string `json:"watchlistItemId,omitempty"`
	HasWatched      bool    `json:"hasWatched"`
	WatchEventID    *string `json:"watchEventId,omitempty"`
}

var validMediaStateTypes = map[string]bool{"movie": true, "series": true}

// GetMediaState returns watchlist state for movies and series, plus watched state for movies.
func (h *Handlers) GetMediaState(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	mediaType := r.URL.Query().Get("media_type")
	if !validMediaStateTypes[mediaType] {
		httputil.Error(w, http.StatusBadRequest, "invalid media_type")
		return
	}

	mediaID, err := strconv.Atoi(r.URL.Query().Get("media_id"))
	if err != nil || mediaID <= 0 {
		httputil.Error(w, http.StatusBadRequest, "invalid media_id")
		return
	}

	resp := mediaStateResponse{}

	item, err := h.watchlistStore.Check(r.Context(), user.ID, mediaType, mediaID)
	if err != nil {
		log.Error().Err(err).Msg("media-state: watchlist check failed")
	} else if item != nil {
		resp.InWatchlist = true
		resp.WatchlistItemID = &item.ID
	}

	if mediaType == "movie" {
		filters := store.WatchEventFilters{
			MediaType: &mediaType,
			MediaID:   &mediaID,
			Limit:     1,
		}
		events, err := h.watchEventStore.List(r.Context(), user.ID, filters)
		if err != nil {
			log.Error().Err(err).Msg("media-state: watch events query failed")
		} else if len(events) > 0 {
			resp.HasWatched = true
			resp.WatchEventID = &events[0].ID
		}
	}

	httputil.JSON(w, http.StatusOK, resp, 0)
}
