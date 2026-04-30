package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAchievements_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.achievementStore.achievements = []store.UserAchievement{
		{ID: "ua-1", AchievementID: "opening_credits", EarnedAt: time.Now()},
		{ID: "ua-2", AchievementID: "century_club", EarnedAt: time.Now()},
	}

	r := authedRequest(t, http.MethodGet, "/achievements", nil)
	w := httptest.NewRecorder()

	td.handlers.GetAchievements(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	achievements := resp["achievements"].([]any)
	assert.Len(t, achievements, 2)

	first := achievements[0].(map[string]any)
	assert.Equal(t, "opening_credits", first["achievementId"])
	assert.Equal(t, "Opening Credits", first["name"])
	assert.Equal(t, "Watch your first movie", first["description"])
}

func TestGetUnseenAchievements_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.achievementStore.unseen = []store.UserAchievement{
		{ID: "ua-3", AchievementID: "genre_explorer:bronze", EarnedAt: time.Now()},
	}

	r := authedRequest(t, http.MethodGet, "/achievements/unseen", nil)
	w := httptest.NewRecorder()

	td.handlers.GetUnseenAchievements(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)
	achievements := resp["achievements"].([]any)
	assert.Len(t, achievements, 1)

	first := achievements[0].(map[string]any)
	assert.Equal(t, "Genre Explorer (Bronze)", first["name"])
}

func TestMarkAchievementsSeen_Success(t *testing.T) {
	td := newTestHandlers(t)

	r := authedRequest(t, http.MethodPost, "/achievements/seen", nil)
	w := httptest.NewRecorder()

	td.handlers.MarkAchievementsSeen(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
}
