package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
)

type deleteAccountRequest struct {
	Confirm string `json:"confirm"`
}

func (h *Handlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Confirm != "delete" {
		httputil.Error(w, http.StatusBadRequest, "confirmation required")
		return
	}

	accounts, err := h.userStore.ListOAuthAccounts(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to load oauth accounts for revocation")
	}

	for _, acct := range accounts {
		if acct.EncryptedRefreshToken == nil || len(h.cfg.TokenEncryptionKey) == 0 {
			continue
		}

		tokenBytes, err := auth.Decrypt(*acct.EncryptedRefreshToken, h.cfg.TokenEncryptionKey)
		if err != nil {
			log.Error().Err(err).Str("provider", acct.Provider).Msg("failed to decrypt refresh token for revocation")
			continue
		}

		if err := h.revokeProviderToken(r.Context(), acct.Provider, string(tokenBytes)); err != nil {
			log.Error().Err(err).Str("provider", acct.Provider).Msg("provider token revocation failed (best-effort)")
		}
	}

	if err := h.userStore.Delete(r.Context(), user.ID); err != nil {
		log.Error().Err(err).Msg("failed to delete user")
		httputil.Error(w, http.StatusInternalServerError, "deletion failed")
		return
	}

	if metrics.M != nil {
		metrics.M.RecordAuthEvent(r.Context(), "any", "account_delete")
	}

	httputil.JSON(w, http.StatusOK, map[string]bool{"deleted": true}, 0)
}

func (h *Handlers) ExportUserData(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	canExport, _ := h.userStore.CanExportToday(r.Context(), user.ID)
	if !canExport {
		httputil.Error(w, http.StatusTooManyRequests, "export limit reached (1 per day)")
		return
	}

	accounts, accErr := h.userStore.ListOAuthAccounts(r.Context(), user.ID)
	if accErr != nil {
		log.Warn().Err(accErr).Msg("export: oauth accounts fetch failed")
	}
	watchlist, wlErr := h.watchlistStore.List(r.Context(), user.ID, nil, nil)
	if wlErr != nil {
		log.Warn().Err(wlErr).Msg("export: watchlist fetch failed")
	}
	events, evErr := h.watchEventStore.List(r.Context(), user.ID, store.WatchEventFilters{Limit: 500})
	if evErr != nil {
		log.Warn().Err(evErr).Msg("export: watch events fetch failed")
	}
	achievements, achErr := h.achievementStore.ListByUserID(r.Context(), user.ID)
	if achErr != nil {
		log.Warn().Err(achErr).Msg("export: achievements fetch failed")
	}

	if accErr != nil && wlErr != nil && evErr != nil && achErr != nil {
		httputil.Error(w, http.StatusInternalServerError, "export failed")
		return
	}

	providers := make([]string, len(accounts))
	for i, a := range accounts {
		providers[i] = a.Provider
	}

	type exportAccount struct {
		Username  *string  `json:"username"`
		GivenName *string  `json:"givenName,omitempty"`
		AvatarURL *string  `json:"avatarUrl,omitempty"`
		Providers []string `json:"providers"`
		CreatedAt string   `json:"createdAt"`
	}

	type exportData struct {
		Account      exportAccount           `json:"account"`
		Watchlist    []store.WatchlistItem   `json:"watchlist"`
		WatchEvents  []store.WatchEvent      `json:"watchEvents"`
		Achievements []store.UserAchievement `json:"achievements"`
	}

	export := exportData{
		Account: exportAccount{
			Username:  user.Username,
			GivenName: user.GivenName,
			AvatarURL: user.AvatarURL,
			Providers: providers,
			CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
		Watchlist:    watchlist,
		WatchEvents:  events,
		Achievements: achievements,
	}

	body, err := json.Marshal(export)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "export failed")
		return
	}

	_ = h.userStore.MarkExported(r.Context(), user.ID)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="minimovie-data-export.json"`)
	w.Header().Set("Vary", "Cookie")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
