package handlers

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rs/zerolog/log"
)

type appleNotificationPayload struct {
	Payload string `json:"payload"`
}

type appleEvent struct {
	Type string `json:"type"`
	Sub  string `json:"sub"`
}

func (h *Handlers) AppleNotifications(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	var payload appleNotificationPayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Payload == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid payload")
		return
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer("https://appleid.apple.com"),
		jwt.WithAudience(h.cfg.AppleClientID),
	)

	token, err := parser.Parse(payload.Payload, h.appleJWKSKeyFunc)
	if err != nil {
		log.Warn().Err(err).Msg("Apple notification: invalid signature or claims")
		httputil.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "invalid claims")
		return
	}

	iat, _ := claims.GetIssuedAt()
	if iat != nil && math.Abs(time.Since(iat.Time).Minutes()) > 10 {
		log.Warn().Msg("Apple notification: iat too far from current time")
		httputil.Error(w, http.StatusBadRequest, "stale notification")
		return
	}

	jti, _ := claims["jti"].(string)
	if jti == "" {
		jti = payload.Payload[:min(32, len(payload.Payload))]
	}

	if h.notificationStore != nil {
		isNew, err := h.notificationStore.MarkSeen(r.Context(), jti, "apple")
		if err != nil {
			log.Error().Err(err).Msg("Apple notification: failed to deduplicate")
		}
		if !isNew {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	eventsRaw, _ := claims["events"].(string)
	if eventsRaw == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var event appleEvent
	if err := json.Unmarshal([]byte(eventsRaw), &event); err != nil {
		log.Warn().Err(err).Msg("Apple notification: failed to parse events")
		w.WriteHeader(http.StatusOK)
		return
	}

	switch event.Type {
	case "consent-revoked", "account-delete":
		if event.Sub != "" {
			log.Info().Str("type", event.Type).Str("sub", event.Sub).Msg("Apple notification: processing user removal")
			if err := h.userStore.Delete(r.Context(), event.Sub); err != nil {
				log.Error().Err(err).Str("sub", event.Sub).Msg("Apple notification: failed to delete user")
			}
		}
	default:
		log.Info().Str("type", event.Type).Msg("Apple notification: unhandled event type")
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) appleJWKSKeyFunc(token *jwt.Token) (interface{}, error) {
	resp, err := http.Get("https://appleid.apple.com/auth/keys")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jwks jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}

	kid, _ := token.Header["kid"].(string)
	keys := jwks.Key(kid)
	if len(keys) == 0 {
		return nil, jwt.ErrTokenSignatureInvalid
	}

	return keys[0].Key, nil
}
