package handlers

import (
	"net/http"

	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rs/zerolog/log"
)

func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	stats, err := h.statsStore.GetStats(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to compute stats")
		httputil.Error(w, http.StatusInternalServerError, "failed to compute stats")
		return
	}

	httputil.JSON(w, http.StatusOK, stats, 0)
}
