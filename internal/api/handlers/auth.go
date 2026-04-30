package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

type sessionUser struct {
	ID        string  `json:"id"`
	Username  *string `json:"username"`
	GivenName *string `json:"givenName,omitempty"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

func (h *Handlers) BeginAuth(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	relyingParty, ok := h.providers.Get(provider)
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "unknown provider")
		return
	}

	var opts []rp.URLParamOpt
	if provider == "apple" {
		opts = append(opts, rp.WithResponseModeURLParam(oidc.ResponseModeFormPost))
	}

	rp.AuthURLHandler(func() string {
		return uuid.New().String()
	}, relyingParty, opts...)(w, r)
}

func (h *Handlers) AuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	relyingParty, ok := h.providers.Get(provider)
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "unknown provider")
		return
	}

	rp.CodeExchangeHandler(h.handleOAuthSuccess, relyingParty)(w, r)
}

func (h *Handlers) handleOAuthSuccess(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, _ rp.RelyingParty) {
	providerName := chi.URLParam(r, "provider")
	sub := tokens.IDTokenClaims.Subject

	if sub == "" {
		log.Error().Str("provider", providerName).Msg("ID token missing subject claim")
		http.Redirect(w, r, h.cfg.AuthUIBaseURL+"/login?error=auth_failed", http.StatusFound)
		return
	}

	var encryptedRefreshToken string
	if tokens.RefreshToken != "" && len(h.cfg.TokenEncryptionKey) == 32 {
		encrypted, err := auth.Encrypt([]byte(tokens.RefreshToken), h.cfg.TokenEncryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("failed to encrypt refresh token")
		} else {
			encryptedRefreshToken = encrypted
		}
	}

	var avatarURL *string
	if pic := tokens.IDTokenClaims.Picture; pic != "" {
		avatarURL = &pic
	}

	var givenName *string
	if name := tokens.IDTokenClaims.GivenName; name != "" {
		givenName = &name
	}

	// Apple sends user name only on first authorization via form POST body
	if providerName == "apple" && givenName == nil {
		if userJSON := r.FormValue("user"); userJSON != "" {
			var appleUser struct {
				Name struct {
					FirstName string `json:"firstName"`
				} `json:"name"`
			}
			if err := json.Unmarshal([]byte(userJSON), &appleUser); err == nil && appleUser.Name.FirstName != "" {
				givenName = &appleUser.Name.FirstName
			}
		}
	}

	result, err := h.userStore.UpsertFromOAuth(r.Context(), providerName, sub, encryptedRefreshToken, avatarURL, givenName)
	if err != nil {
		log.Error().Err(err).Msg("failed to upsert user from oauth")
		http.Redirect(w, r, h.cfg.AuthUIBaseURL+"/login?error=auth_failed", http.StatusFound)
		return
	}

	if metrics.M != nil {
		event := "login_returning"
		if result.IsNewUser {
			event = "login_new"
		}
		metrics.M.RecordAuthEvent(r.Context(), providerName, event)
	}

	rawToken, err := auth.GenerateToken()
	if err != nil {
		log.Error().Err(err).Msg("failed to generate session token")
		http.Redirect(w, r, h.cfg.AuthUIBaseURL+"/login?error=auth_failed", http.StatusFound)
		return
	}

	tokenHash := auth.HashToken(rawToken, h.cfg.SessionSecret)

	_, err = h.sessionStore.Create(r.Context(), tokenHash, result.User.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to create session")
		http.Redirect(w, r, h.cfg.AuthUIBaseURL+"/login?error=auth_failed", http.StatusFound)
		return
	}

	code, err := h.authCodeStore.Create(r.Context(), tokenHash, result.User.ID, rawToken)
	if err != nil {
		log.Error().Err(err).Msg("failed to create auth code")
		http.Redirect(w, r, h.cfg.AuthUIBaseURL+"/login?error=auth_failed", http.StatusFound)
		return
	}

	redirectURL := h.cfg.AuthUIBaseURL + "/auth/finish?code=" + code
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

type tokenRequest struct {
	Code string `json:"code"`
}

type tokenResponse struct {
	SessionToken string `json:"session_token"`
}

func (h *Handlers) ExchangeToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		httputil.Error(w, http.StatusBadRequest, "missing code")
		return
	}

	authCode, err := h.authCodeStore.Exchange(r.Context(), req.Code)
	if err != nil {
		log.Error().Err(err).Msg("failed to exchange auth code")
		httputil.Error(w, http.StatusInternalServerError, "token exchange failed")
		return
	}
	if authCode == nil {
		httputil.Error(w, http.StatusGone, "code expired or already exchanged")
		return
	}

	if authCode.RawSessionToken == "" {
		httputil.Error(w, http.StatusGone, "session token not available")
		return
	}

	if metrics.M != nil {
		metrics.M.RecordAuthEvent(r.Context(), "any", "token_exchange")
	}

	httputil.JSON(w, http.StatusOK, tokenResponse{SessionToken: authCode.RawSessionToken}, 0)
}

func (h *Handlers) GetSession(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	unseenCount, _ := h.achievementStore.CountUnseen(r.Context(), user.ID)

	type sessionResponse struct {
		User                   *sessionUser `json:"user"`
		UnseenAchievementCount int          `json:"unseenAchievementCount"`
	}

	httputil.JSON(w, http.StatusOK, sessionResponse{
		User: &sessionUser{
			ID:        user.ID,
			Username:  user.Username,
			GivenName: user.GivenName,
			AvatarURL: user.AvatarURL,
		},
		UnseenAchievementCount: unseenCount,
	}, 0)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	sessionHash := getSessionHashFromContext(r.Context())
	if sessionHash == "" {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.sessionStore.Delete(r.Context(), sessionHash); err != nil {
		log.Error().Err(err).Msg("failed to delete session")
	}

	if metrics.M != nil {
		metrics.M.RecordAuthEvent(r.Context(), "any", "logout")
	}

	w.WriteHeader(http.StatusNoContent)
}
