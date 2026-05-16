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

var validWatchEventMediaTypes = map[string]bool{"movie": true, "episode": true, "season": true}

type createWatchEventRequest struct {
	MediaType     string `json:"mediaType"`
	MediaID       int    `json:"mediaId"`
	SeriesID      *int   `json:"seriesId,omitempty"`
	SeasonNumber  *int   `json:"seasonNumber,omitempty"`
	EpisodeNumber *int   `json:"episodeNumber,omitempty"`
	JustWatched   bool   `json:"justWatched"`
	Timezone      string `json:"timezone"`
}

type progressSeasonEvent struct {
	ID           string    `json:"id"`
	SeasonNumber int       `json:"seasonNumber"`
	EpisodeCount int       `json:"episodeCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

type progressEpisodeEvent struct {
	ID            string    `json:"id"`
	SeasonNumber  int       `json:"seasonNumber"`
	EpisodeNumber int       `json:"episodeNumber"`
	CreatedAt     time.Time `json:"createdAt"`
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

	key := user.ID + "-" + req.MediaType + "-" + strconv.Itoa(req.MediaID)
	watchEventId := uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()

	// Resolve the watchlist item ID synchronously
	lookupType, lookupId := getWatchlistTarget(req.MediaType, req.MediaID, req.SeriesID)
	watchlistItemId, needsWatchlistCreate, err := h.getWatchlistItemId(r.Context(), user.ID, lookupType, lookupId)
	if err != nil {
		log.Error().Err(err).Msg("failed to resolve watchlist item id")
		httputil.Error(w, http.StatusInternalServerError, "failed to record watch event")
		return
	}

	go h.processWatchEvent(user.ID, req, tz, watchEventId, watchlistItemId, needsWatchlistCreate)

	httputil.JSON(w, http.StatusAccepted, map[string]string{
		"id":              watchEventId,
		"watchlistItemId": watchlistItemId,
	}, 0)
}

// getWatchlistItemId returns the watchlist item ID for the given target
// from the DB if already present, otherwise generates a new deterministic UUID
func (h *Handlers) getWatchlistItemId(ctx context.Context, userId, mediaType string, mediaID int) (string, bool, error) {
	existing, err := h.watchlistStore.Check(ctx, userId, mediaType, mediaID)
	if err != nil {
		return "", false, err
	}
	if existing != nil {
		return existing.ID, false, nil
	}
	key := "watchlist-" + userId + "-" + mediaType + "-" + strconv.Itoa(mediaID)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String(), true, nil
}

func validateCreateWatchEvent(req createWatchEventRequest) string {
	if req.MediaType == "series" {
		return "series-level watch events are not supported; mark progress per season or episode"
	}
	if !validWatchEventMediaTypes[req.MediaType] {
		return "mediaType must be 'movie', 'episode', or 'season'"
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

func (h *Handlers) processWatchEvent(userId string, req createWatchEventRequest, tz string, watchEventId string, watchlistItemId string, needsWatchlistCreate bool) {
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

	input := store.WatchEventCreate{
		ID:            watchEventId,
		UserID:        userId,
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
	}

	// MarkSeason transactionally clears prior per-episode rows so the
	// watchlist SUM + COUNT aggregation never double-counts. Other types
	// go through plain UPSERT.
	writeFn := h.watchEventStore.Create
	if req.MediaType == "season" {
		writeFn = h.watchEventStore.MarkSeason
	}
	if _, err := writeFn(ctx, input); err != nil {
		log.Error().Err(err).Str("mediaType", req.MediaType).Int("mediaId", req.MediaID).Msg("failed to create watch event")
		return
	}

	lookupType, lookupID := getWatchlistTarget(req.MediaType, req.MediaID, req.SeriesID)
	if needsWatchlistCreate {
		h.createWatchlistFromEvent(ctx, watchlistItemId, userId, req.MediaType, lookupType, lookupID, meta)
	}
	if err := h.watchlistStore.UpdateSummary(ctx, userId, lookupType, lookupID); err != nil {
		log.Warn().Err(err).Str("mediaType", lookupType).Int("mediaId", lookupID).Msg("watchlist UpdateSummary failed")
	}
	h.trackWatchEvent(ctx, userId, "create", req.MediaType)
}

// createWatchlistFromEvent inserts a watchlist row using the caller-supplied
// ID. For season/episode events we re-resolve series-level metadata so the
// row has the right title/poster for the user's list view. A unique-violation
// here is benign (a concurrent goroutine for the same user+media won the race)
// and surfaces only as a warning log.
func (h *Handlers) createWatchlistFromEvent(ctx context.Context, id, userId, eventMediaType, lookupType string, lookupID int, eventMeta store.ResolvedMedia) {
	syncMeta := eventMeta
	if lookupType != eventMediaType {
		resolved, err := h.tmdbResolver.ResolveSeries(ctx, lookupID)
		if err != nil {
			log.Warn().Err(err).Int("seriesId", lookupID).Msg("series re-resolve failed during watchlist sync; using event meta")
		} else {
			syncMeta = resolved
		}
	}
	if _, err := h.watchlistStore.Create(ctx, id, userId, lookupType, lookupID, "watched", syncMeta); err != nil {
		log.Warn().Err(err).Str("mediaType", lookupType).Int("mediaId", lookupID).Msg("watchlist auto-add failed")
	}
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

	seriesIdStr := chi.URLParam(r, "seriesId")
	seriesId, err := strconv.Atoi(seriesIdStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid seriesId")
		return
	}

	events, err := h.watchEventStore.ListBySeriesID(r.Context(), user.ID, seriesId)
	if err != nil {
		log.Error().Err(err).Msg("failed to get watch progress")
		httputil.Error(w, http.StatusInternalServerError, "failed to get progress")
		return
	}

	seasons := make([]progressSeasonEvent, 0)
	episodes := make([]progressEpisodeEvent, 0)
	for _, ev := range events {
		switch ev.MediaType {
		case "season":
			if ev.SeasonNumber == nil {
				continue
			}
			ec := 0
			if ev.EpisodeCount != nil {
				ec = *ev.EpisodeCount
			}
			seasons = append(seasons, progressSeasonEvent{
				ID:           ev.ID,
				SeasonNumber: *ev.SeasonNumber,
				EpisodeCount: ec,
				CreatedAt:    ev.CreatedAt,
			})
		case "episode":
			if ev.SeasonNumber == nil || ev.EpisodeNumber == nil {
				continue
			}
			episodes = append(episodes, progressEpisodeEvent{
				ID:            ev.ID,
				SeasonNumber:  *ev.SeasonNumber,
				EpisodeNumber: *ev.EpisodeNumber,
				CreatedAt:     ev.CreatedAt,
			})
		}
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"seriesId":      seriesId,
		"seasonEvents":  seasons,
		"episodeEvents": episodes,
	}, 0)
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

func (h *Handlers) resolveMetadata(ctx context.Context, mediaType string, mediaId int, seriesId, seasonNumber, episodeNumber *int) (store.ResolvedMedia, error) {
	switch mediaType {
	case "movie":
		return h.tmdbResolver.ResolveMovie(ctx, mediaId)
	case "series":
		return h.tmdbResolver.ResolveSeries(ctx, mediaId)
	case "season":
		if seriesId == nil || seasonNumber == nil {
			return store.ResolvedMedia{}, nil
		}
		return h.tmdbResolver.ResolveSeason(ctx, *seriesId, *seasonNumber)
	case "episode":
		if seriesId == nil || seasonNumber == nil || episodeNumber == nil {
			return store.ResolvedMedia{}, nil
		}
		return h.tmdbResolver.ResolveEpisode(ctx, *seriesId, *seasonNumber, *episodeNumber)
	default:
		return store.ResolvedMedia{}, nil
	}
}

func (h *Handlers) resolveRuntimeFromSeasons(ctx context.Context, mediaType string, mediaId int, seriesId, seasonNumber *int) int {
	switch mediaType {
	case "series":
		seriesWithSeasons, err := h.tmdbClient.GetSeriesWithSeasons(ctx, mediaId)
		if err != nil {
			log.Warn().Err(err).Int("mediaId", mediaId).Msg("failed to fetch series with seasons for runtime")
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
		if seriesId == nil || seasonNumber == nil {
			return 0
		}
		season, err := h.tmdbClient.GetSeason(ctx, *seriesId, *seasonNumber)
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

func getWatchlistTarget(mediaType string, mediaId int, seriesId *int) (string, int) {
	switch mediaType {
	case "episode", "season":
		if seriesId != nil {
			return "series", *seriesId
		}
	}
	return mediaType, mediaId
}

func (h *Handlers) syncWatchlistAfterDelete(ctx context.Context, userId string, event *store.WatchEvent) {
	if event.SeriesID != nil {
		_ = h.watchlistStore.UpdateSummary(ctx, userId, "series", *event.SeriesID)
	} else if event.MediaType == "movie" {
		_ = h.watchlistStore.UpdateSummary(ctx, userId, "movie", event.MediaID)
	}
}

func (h *Handlers) trackWatchEvent(ctx context.Context, userId, action, mediaType string) {
	if h.achievementWorker != nil {
		h.achievementWorker.Enqueue(userId)
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
