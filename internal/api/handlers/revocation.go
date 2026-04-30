package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/auth"
)

func (h *Handlers) revokeProviderToken(ctx context.Context, provider, refreshToken string) error {
	switch provider {
	case "google":
		return revokeGoogleToken(ctx, refreshToken)
	case "apple":
		if h.cfg.AppleClientID != "" {
			secret, _ := generateAppleSecret(h.cfg)
			return revokeAppleToken(ctx, refreshToken, h.cfg.AppleClientID, secret)
		}
		return nil
	default:
		return nil
	}
}

func revokeGoogleToken(ctx context.Context, refreshToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/revoke",
		strings.NewReader("token="+refreshToken))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func revokeAppleToken(ctx context.Context, refreshToken, clientID, clientSecret string) error {
	body := "client_id=" + clientID + "&client_secret=" + clientSecret +
		"&token=" + refreshToken + "&token_type_hint=refresh_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://appleid.apple.com/auth/revoke",
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func generateAppleSecret(cfg *config.Config) (string, error) {
	if len(cfg.ApplePrivateKey) == 0 {
		return "", nil
	}
	return auth.GenerateAppleClientSecret(cfg.AppleTeamID, cfg.AppleKeyID, cfg.AppleClientID, cfg.ApplePrivateKey)
}
