package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type addWatchlistRequest struct {
	MediaType string `json:"mediaType"`
	MediaID   int    `json:"mediaId"`
	Status    string `json:"status"`
}

func (h *Handlers) ListWatchlist(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var status, mediaType *string
	if s := r.URL.Query().Get("status"); s != "" {
		status = &s
	}
	if mt := r.URL.Query().Get("media_type"); mt != "" {
		mediaType = &mt
	}

	items, err := h.watchlistStore.List(r.Context(), user.ID, status, mediaType)
	if err != nil {
		log.Error().Err(err).Msg("failed to list watchlist")
		httputil.Error(w, http.StatusInternalServerError, "failed to list watchlist")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)}, 0)
}

func (h *Handlers) AddToWatchlist(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req addWatchlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !isValidMediaType(req.MediaType) {
		httputil.Error(w, http.StatusBadRequest, "mediaType must be 'movie' or 'series'")
		return
	}
	if !isValidWatchlistStatus(req.Status) {
		httputil.Error(w, http.StatusBadRequest, "status must be 'want_to_watch' or 'watched'")
		return
	}
	if req.MediaID <= 0 {
		httputil.Error(w, http.StatusBadRequest, "mediaId must be a positive integer")
		return
	}

	meta, err := h.resolveMedia(r.Context(), req.MediaType, req.MediaID)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "media not found on TMDB")
			return
		}
		log.Error().Err(err).Msg("TMDB resolution failed")
		httputil.Error(w, http.StatusServiceUnavailable, "metadata resolution failed")
		return
	}

	item, err := h.watchlistStore.Create(r.Context(), uuid.New().String(), user.ID, req.MediaType, req.MediaID, req.Status, meta)
	if err != nil {
		if isUniqueViolation(err) {
			httputil.Error(w, http.StatusConflict, "item already in watchlist")
			return
		}
		log.Error().Err(err).Msg("failed to add to watchlist")
		httputil.Error(w, http.StatusInternalServerError, "failed to add to watchlist")
		return
	}

	if h.achievementWorker != nil {
		h.achievementWorker.Enqueue(user.ID)
	}

	if metrics.M != nil {
		metrics.M.RecordWatchlistOperation(r.Context(), "add", req.MediaType)
	}

	httputil.JSON(w, http.StatusCreated, item, 0)
}

func (h *Handlers) UpdateWatchlistStatus(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !isValidWatchlistStatus(req.Status) {
		httputil.Error(w, http.StatusBadRequest, "status must be 'want_to_watch' or 'watched'")
		return
	}

	item, err := h.watchlistStore.UpdateStatus(r.Context(), id, user.ID, req.Status)
	if err != nil {
		log.Error().Err(err).Msg("failed to update watchlist item")
		httputil.Error(w, http.StatusInternalServerError, "failed to update watchlist item")
		return
	}
	if item == nil {
		httputil.Error(w, http.StatusNotFound, "item not found")
		return
	}

	if h.achievementWorker != nil {
		h.achievementWorker.Enqueue(user.ID)
	}

	if metrics.M != nil {
		metrics.M.RecordWatchlistOperation(r.Context(), "update_status", item.MediaType)
	}

	httputil.JSON(w, http.StatusOK, item, 0)
}

func (h *Handlers) RemoveFromWatchlist(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.watchlistStore.Delete(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.Error(w, http.StatusNotFound, "item not found")
			return
		}
		log.Error().Err(err).Msg("failed to remove from watchlist")
		httputil.Error(w, http.StatusInternalServerError, "failed to remove")
		return
	}

	if metrics.M != nil {
		metrics.M.RecordWatchlistOperation(r.Context(), "remove", "any")
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) CheckWatchlist(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	mediaType := r.URL.Query().Get("media_type")
	mediaIDStr := r.URL.Query().Get("media_id")
	mediaID, err := strconv.Atoi(mediaIDStr)
	if err != nil || mediaID <= 0 {
		httputil.Error(w, http.StatusBadRequest, "invalid media_id")
		return
	}

	item, err := h.watchlistStore.Check(r.Context(), user.ID, mediaType, mediaID)
	if err != nil {
		log.Error().Err(err).Msg("failed to check watchlist")
		httputil.Error(w, http.StatusInternalServerError, "check failed")
		return
	}

	if item == nil {
		httputil.JSON(w, http.StatusOK, map[string]any{"exists": false}, 0)
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"exists": true,
		"item":   map[string]any{"id": item.ID, "status": item.Status},
	}, 0)
}

func isValidMediaType(mt string) bool {
	return mt == "movie" || mt == "series"
}

func isValidWatchlistStatus(s string) bool {
	return s == "want_to_watch" || s == "watched"
}
