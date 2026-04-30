package handlers

import (
	"net/http"

	"github.com/rossbrandon/minimovie-api/internal/achievements"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
)

type achievementResponse struct {
	ID                  string  `json:"id"`
	AchievementID       string  `json:"achievementId"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	EarnedViaMediaType  *string `json:"earnedViaMediaType,omitempty"`
	EarnedViaMediaID    *int    `json:"earnedViaMediaId,omitempty"`
	EarnedViaMediaTitle *string `json:"earnedViaMediaTitle,omitempty"`
	SeenAt              *string `json:"seenAt,omitempty"`
	EarnedAt            string  `json:"earnedAt"`
}

func (h *Handlers) GetAchievements(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.achievementStore.ListByUserID(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to list achievements")
		httputil.Error(w, http.StatusInternalServerError, "failed to list achievements")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"achievements": enrichAchievements(rows)}, 0)
}

func (h *Handlers) GetUnseenAchievements(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.achievementStore.ListUnseen(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to list unseen achievements")
		httputil.Error(w, http.StatusInternalServerError, "failed to list unseen achievements")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"achievements": enrichAchievements(rows)}, 0)
}

func (h *Handlers) MarkAchievementsSeen(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.achievementStore.MarkAllSeen(r.Context(), user.ID); err != nil {
		log.Error().Err(err).Msg("failed to mark achievements seen")
		httputil.Error(w, http.StatusInternalServerError, "failed to mark achievements seen")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func enrichAchievements(rows []store.UserAchievement) []achievementResponse {
	result := make([]achievementResponse, 0, len(rows))
	for _, a := range rows {
		resp := achievementResponse{
			ID:                  a.ID,
			AchievementID:       a.AchievementID,
			EarnedViaMediaType:  a.EarnedViaMediaType,
			EarnedViaMediaID:    a.EarnedViaMediaID,
			EarnedViaMediaTitle: a.EarnedViaMediaTitle,
			EarnedAt:            a.EarnedAt.Format("2006-01-02T15:04:05Z"),
		}
		if a.SeenAt != nil {
			s := a.SeenAt.Format("2006-01-02T15:04:05Z")
			resp.SeenAt = &s
		}
		if def, ok := achievements.GetDefinition(a.AchievementID); ok {
			resp.Name = def.Name
			resp.Description = def.Description
		}
		result = append(result, resp)
	}
	return result
}
