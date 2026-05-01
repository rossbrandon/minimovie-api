package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
)

var validWatchEventMediaTypes = map[string]bool{"movie": true, "episode": true, "series": true, "season": true}

type createWatchEventRequest struct {
	MediaType     string `json:"mediaType"`
	MediaID       int    `json:"mediaId"`
	SeriesID      *int   `json:"seriesId,omitempty"`
	SeasonNumber  *int   `json:"seasonNumber,omitempty"`
	EpisodeNumber *int   `json:"episodeNumber,omitempty"`
	JustWatched   bool   `json:"justWatched"`
	Timezone      string `json:"timezone"`
}

type markEpisodeRequest struct {
	SeriesID      int    `json:"seriesId"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Timezone      string `json:"timezone"`
}

func (h *Handlers) CreateWatchEvent(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createWatchEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateCreateWatchEvent(req); err != "" {
		httputil.Error(w, http.StatusBadRequest, err)
		return
	}

	tz, tzErr := normalizeTimezone(req.Timezone)
	if tzErr != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid timezone")
		return
	}

	watchEventID := uuid.New().String()
	go h.processWatchEvent(user.ID, req, tz, watchEventID)

	httputil.JSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "id": watchEventID}, 0)
}

func validateCreateWatchEvent(req createWatchEventRequest) string {
	if !validWatchEventMediaTypes[req.MediaType] {
		return "mediaType must be 'movie', 'episode', 'series', or 'season'"
	}
	if req.MediaID <= 0 {
		return "mediaId must be positive"
	}
	switch req.MediaType {
	case "season":
		if req.SeriesID == nil || req.SeasonNumber == nil {
			return "seriesId and seasonNumber required for season"
		}
	case "episode":
		if req.SeriesID == nil || req.SeasonNumber == nil || req.EpisodeNumber == nil {
			return "seriesId, seasonNumber, episodeNumber required for episode"
		}
	}
	return ""
}

func (h *Handlers) processWatchEvent(userID string, req createWatchEventRequest, tz string, watchEventID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	meta, err := h.resolveMetadata(ctx, req.MediaType, req.MediaID, req.SeriesID, req.SeasonNumber, req.EpisodeNumber)
	if err != nil {
		log.Error().Err(err).Str("mediaType", req.MediaType).Int("mediaId", req.MediaID).Msg("failed to resolve metadata for watch event")
		return
	}

	if meta.RuntimeMinutes == nil && (req.MediaType == "series" || req.MediaType == "season") {
		runtime := h.resolveRuntimeFromSeasons(ctx, req.MediaType, req.MediaID, req.SeriesID, req.SeasonNumber)
		if runtime > 0 {
			meta.RuntimeMinutes = &runtime
		}
	}

	var watchedAt *time.Time
	if req.JustWatched {
		now := time.Now()
		watchedAt = &now
	}

	_, err = h.watchEventStore.Create(ctx, store.WatchEventCreate{
		ID:            watchEventID,
		UserID:        userID,
		MediaType:     req.MediaType,
		MediaID:       req.MediaID,
		SeriesID:      req.SeriesID,
		SeasonNumber:  req.SeasonNumber,
		EpisodeNumber: req.EpisodeNumber,
		EpisodeCount:  meta.EpisodeCount,
		SeasonCount:   meta.SeasonCount,
		WatchedAt:     watchedAt,
		Timezone:      tz,
		Meta:          meta,
	})
	if err != nil {
		log.Error().Err(err).Str("mediaType", req.MediaType).Int("mediaId", req.MediaID).Msg("failed to create watch event")
		return
	}

	h.syncWatchlistItem(ctx, userID, req.MediaType, req.MediaID, req.SeriesID, meta)
	h.trackWatchEvent(ctx, userID, "create", req.MediaType)
}

func (h *Handlers) ListWatchEvents(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	filters := store.WatchEventFilters{Limit: 100}
	if mt := r.URL.Query().Get("media_type"); mt != "" {
		filters.MediaType = &mt
	}
	if mid := r.URL.Query().Get("media_id"); mid != "" {
		if id, err := strconv.Atoi(mid); err == nil {
			filters.MediaID = &id
		}
	}
	if sid := r.URL.Query().Get("series_id"); sid != "" {
		if id, err := strconv.Atoi(sid); err == nil {
			filters.SeriesID = &id
		}
	}
	if r.URL.Query().Get("dated_only") == "true" {
		filters.DatedOnly = true
	}
	if lim := r.URL.Query().Get("limit"); lim != "" {
		if l, err := strconv.Atoi(lim); err == nil {
			filters.Limit = l
		}
	}

	events, err := h.watchEventStore.List(r.Context(), user.ID, filters)
	if err != nil {
		log.Error().Err(err).Msg("failed to list watch events")
		httputil.Error(w, http.StatusInternalServerError, "failed to list watch events")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"events": events, "total": len(events)}, 0)
}

func (h *Handlers) GetWatchProgress(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	seriesIDStr := chi.URLParam(r, "seriesId")
	seriesID, err := strconv.Atoi(seriesIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid seriesId")
		return
	}

	events, err := h.watchEventStore.ListBySeriesID(r.Context(), user.ID, seriesID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get watch progress")
		httputil.Error(w, http.StatusInternalServerError, "failed to get progress")
		return
	}

	type episodeProgress struct {
		ID            string     `json:"id"`
		SeasonNumber  *int       `json:"seasonNumber"`
		EpisodeNumber *int       `json:"episodeNumber"`
		WatchedAt     *time.Time `json:"watchedAt,omitempty"`
	}

	episodes := make([]episodeProgress, 0, len(events))
	for _, ev := range events {
		episodes = append(episodes, episodeProgress{
			ID:            ev.ID,
			SeasonNumber:  ev.SeasonNumber,
			EpisodeNumber: ev.EpisodeNumber,
			WatchedAt:     ev.WatchedAt,
		})
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"seriesId": seriesID,
		"episodes": episodes,
		"total":    len(episodes),
	}, 0)
}

func (h *Handlers) MarkEpisodeWatched(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req markEpisodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SeriesID <= 0 || req.SeasonNumber <= 0 || req.EpisodeNumber <= 0 {
		httputil.Error(w, http.StatusBadRequest, "seriesId, seasonNumber, and episodeNumber must be positive")
		return
	}

	tz, _ := normalizeTimezone(req.Timezone)

	go h.processEpisodeWatch(user.ID, req, tz)

	httputil.JSON(w, http.StatusAccepted, map[string]string{"status": "accepted"}, 0)
}

func (h *Handlers) processEpisodeWatch(userID string, req markEpisodeRequest, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	meta, err := h.tmdbResolver.ResolveEpisode(ctx, req.SeriesID, req.SeasonNumber, req.EpisodeNumber)
	if err != nil {
		log.Error().Err(err).Int("seriesId", req.SeriesID).Int("season", req.SeasonNumber).Int("episode", req.EpisodeNumber).Msg("failed to resolve episode metadata")
		return
	}

	mediaID := 0
	if meta.MediaID != nil {
		mediaID = *meta.MediaID
	}

	seriesID := req.SeriesID
	seasonNum := req.SeasonNumber
	episodeNum := req.EpisodeNumber
	now := time.Now()

	_, err = h.watchEventStore.Create(ctx, store.WatchEventCreate{
		UserID:        userID,
		MediaType:     "episode",
		MediaID:       mediaID,
		SeriesID:      &seriesID,
		SeasonNumber:  &seasonNum,
		EpisodeNumber: &episodeNum,
		WatchedAt:     &now,
		Timezone:      tz,
		Meta:          meta,
	})
	if err != nil {
		log.Error().Err(err).Int("seriesId", req.SeriesID).Msg("failed to create episode watch event")
		return
	}

	h.syncWatchlistItem(ctx, userID, "episode", mediaID, &seriesID, meta)
	h.trackWatchEvent(ctx, userID, "create", "episode")
}

func (h *Handlers) UnmarkEpisode(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")

	event, err := h.watchEventStore.GetByID(r.Context(), id, user.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get watch event for unmark")
		httputil.Error(w, http.StatusInternalServerError, "failed to unmark")
		return
	}
	if event == nil {
		httputil.Error(w, http.StatusNotFound, "event not found")
		return
	}

	if err := h.watchEventStore.Delete(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.Error(w, http.StatusNotFound, "event not found")
			return
		}
		log.Error().Err(err).Msg("failed to unmark episode")
		httputil.Error(w, http.StatusInternalServerError, "failed to unmark")
		return
	}

	h.syncWatchlistAfterDelete(r.Context(), user.ID, event)
	h.trackWatchEvent(r.Context(), user.ID, "delete", event.MediaType)

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) resolveMetadata(ctx context.Context, mediaType string, mediaID int, seriesID, seasonNumber, episodeNumber *int) (store.ResolvedMedia, error) {
	switch mediaType {
	case "movie":
		return h.tmdbResolver.ResolveMovie(ctx, mediaID)
	case "series":
		return h.tmdbResolver.ResolveSeries(ctx, mediaID)
	case "season":
		if seriesID == nil || seasonNumber == nil {
			return store.ResolvedMedia{}, nil
		}
		return h.tmdbResolver.ResolveSeason(ctx, *seriesID, *seasonNumber)
	case "episode":
		if seriesID == nil || seasonNumber == nil || episodeNumber == nil {
			return store.ResolvedMedia{}, nil
		}
		return h.tmdbResolver.ResolveEpisode(ctx, *seriesID, *seasonNumber, *episodeNumber)
	default:
		return store.ResolvedMedia{}, nil
	}
}

func (h *Handlers) resolveRuntimeFromSeasons(ctx context.Context, mediaType string, mediaID int, seriesID, seasonNumber *int) int {
	switch mediaType {
	case "series":
		seriesWithSeasons, err := h.tmdbClient.GetSeriesWithSeasons(ctx, mediaID)
		if err != nil {
			log.Warn().Err(err).Int("mediaId", mediaID).Msg("failed to fetch series with seasons for runtime")
			return 0
		}
		var total int
		for _, season := range seriesWithSeasons.SeasonDetails {
			for _, ep := range season.Episodes {
				total += ep.Runtime
			}
		}
		return total
	case "season":
		if seriesID == nil || seasonNumber == nil {
			return 0
		}
		season, err := h.tmdbClient.GetSeason(ctx, *seriesID, *seasonNumber)
		if err != nil {
			log.Warn().Err(err).Msg("failed to fetch season for runtime")
			return 0
		}
		var total int
		for _, ep := range season.Episodes {
			total += ep.Runtime
		}
		return total
	}
	return 0
}

func (h *Handlers) syncWatchlistItem(ctx context.Context, userID, mediaType string, mediaID int, seriesID *int, meta store.ResolvedMedia) {
	lookupType, lookupID := resolveWatchlistTarget(mediaType, mediaID, seriesID)
	existing, _ := h.watchlistStore.Check(ctx, userID, lookupType, lookupID)
	if existing == nil {
		_, _ = h.watchlistStore.Create(ctx, userID, lookupType, lookupID, "watched", meta)
	} else {
		_ = h.watchlistStore.UpdateSummary(ctx, userID, lookupType, lookupID)
	}
}

func resolveWatchlistTarget(mediaType string, mediaID int, seriesID *int) (string, int) {
	switch mediaType {
	case "episode", "season":
		if seriesID != nil {
			return "series", *seriesID
		}
	}
	return mediaType, mediaID
}

func (h *Handlers) syncWatchlistAfterDelete(ctx context.Context, userID string, event *store.WatchEvent) {
	if event.SeriesID != nil {
		_ = h.watchlistStore.UpdateSummary(ctx, userID, "series", *event.SeriesID)
	} else if event.MediaType == "movie" {
		_ = h.watchlistStore.UpdateSummary(ctx, userID, "movie", event.MediaID)
	}
}

func (h *Handlers) trackWatchEvent(ctx context.Context, userID, action, mediaType string) {
	if h.achievementWorker != nil {
		h.achievementWorker.Enqueue(userID)
	}
	if metrics.M != nil {
		metrics.M.RecordWatchEvent(ctx, action, mediaType)
	}
}

func normalizeTimezone(tz string) (string, error) {
	if tz == "" {
		return "UTC", nil
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return "", err
	}
	return tz, nil
}
