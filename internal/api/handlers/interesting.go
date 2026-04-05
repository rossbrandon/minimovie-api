package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rs/zerolog/log"
)

func (h *Handlers) GetPersonInterestingInfo(w http.ResponseWriter, r *http.Request) {
	if h.augurResolver == nil {
		httputil.Error(w, http.StatusServiceUnavailable, "Person Interesting Info is not available")
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid person ID")
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		httputil.Error(w, http.StatusBadRequest, "name query parameter is required")
		return
	}

	bypassCache := r.Header.Get("MM-Cache-Bypass") == "true"

	info, err := h.augurResolver.GetPersonInsights(r.Context(), id, name, bypassCache)
	if err != nil {
		log.Error().Err(err).Int("person_id", id).Msg("failed to fetch person insights")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch person interesting info")
		return
	}

	httputil.JSON(w, http.StatusOK, info)
}
