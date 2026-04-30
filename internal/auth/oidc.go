package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
)

type ProviderRegistry interface {
	Get(name string) (rp.RelyingParty, bool)
	Allowed(name string) bool
}

type ProviderConfig struct {
	GoogleIssuerURL    string
	GoogleClientID     string
	GoogleClientSecret string
	AppleIssuerURL     string
	AppleClientID      string
	AppleTeamID        string
	AppleKeyID         string
	ApplePrivateKey    []byte
	BaseURL            string
	CookieHashKey      []byte
	CookieEncKey       []byte
	IsProduction       bool
}

type OIDCProviders struct {
	providers map[string]rp.RelyingParty
}

func NewOIDCProviders(ctx context.Context, cfg ProviderConfig) (*OIDCProviders, error) {
	cookieOpts := []httphelper.CookieHandlerOpt{
		httphelper.WithSameSite(http.SameSiteLaxMode),
	}
	if !cfg.IsProduction {
		cookieOpts = append(cookieOpts, httphelper.WithUnsecure())
	}
	cookieHandler := httphelper.NewCookieHandler(cfg.CookieHashKey, cfg.CookieEncKey, cookieOpts...)

	// Apple uses response_mode=form_post which is a cross-origin POST from appleid.apple.com,
	// so it needs SameSite=None + Secure (Secure is the default without WithUnsecure).
	appleCookieHandler := httphelper.NewCookieHandler(cfg.CookieHashKey, cfg.CookieEncKey,
		httphelper.WithSameSite(http.SameSiteNoneMode),
	)

	providers := make(map[string]rp.RelyingParty)

	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		google, err := rp.NewRelyingPartyOIDC(ctx,
			cfg.GoogleIssuerURL,
			cfg.GoogleClientID,
			cfg.GoogleClientSecret,
			cfg.BaseURL+"/auth/google/callback",
			[]string{"openid", "profile"},
			rp.WithCookieHandler(cookieHandler),
			rp.WithPKCE(cookieHandler),
			rp.WithSigningAlgsFromDiscovery(),
		)
		if err != nil {
			return nil, fmt.Errorf("creating Google OIDC provider: %w", err)
		}
		providers["google"] = google
		log.Info().Msg("Google OIDC provider registered")
	}

	if cfg.AppleClientID != "" && len(cfg.ApplePrivateKey) > 0 {
		secret, err := GenerateAppleClientSecret(cfg.AppleTeamID, cfg.AppleKeyID, cfg.AppleClientID, cfg.ApplePrivateKey)
		if err != nil {
			return nil, fmt.Errorf("generating Apple client secret: %w", err)
		}
		apple, err := rp.NewRelyingPartyOIDC(ctx,
			cfg.AppleIssuerURL,
			cfg.AppleClientID,
			secret,
			cfg.BaseURL+"/auth/apple/callback",
			[]string{"name"},
			rp.WithCookieHandler(appleCookieHandler),
			rp.WithPKCE(appleCookieHandler),
			rp.WithSigningAlgsFromDiscovery(),
		)
		if err != nil {
			return nil, fmt.Errorf("creating Apple OIDC provider: %w", err)
		}
		providers["apple"] = apple
		log.Info().Msg("Apple OIDC provider registered")
	}

	if len(providers) == 0 {
		log.Warn().Msg("No OIDC providers configured - auth endpoints will not function")
	}

	return &OIDCProviders{providers: providers}, nil
}

func (p *OIDCProviders) Get(name string) (rp.RelyingParty, bool) {
	provider, ok := p.providers[name]
	return provider, ok
}

func (p *OIDCProviders) Allowed(name string) bool {
	_, ok := p.providers[name]
	return ok
}
