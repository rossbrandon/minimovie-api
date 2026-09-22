package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Setenv("TMDB_ACCESS_TOKEN", "token")
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("TIMEOUT", "7")
	t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 32))))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 7, cfg.Timeout)
	assert.Equal(t, defaultTmdbTimeout, cfg.TmdbTimeout, "unset keeps the default")
	assert.Len(t, cfg.CookieHashKey, 16)
	assert.Len(t, cfg.CookieEncKey, 16)

	t.Setenv("TIMEOUT", "seven")
	t.Setenv("TMDB_RATE_LIMIT", "0")
	t.Setenv("DATABASE_URL", "")
	_, err = Load()
	require.Error(t, err)
	for _, want := range []string{"TIMEOUT", "TMDB_RATE_LIMIT: not a positive number", "DATABASE_URL is not set"} {
		assert.ErrorContains(t, err, want, "every problem is reported at once")
	}
}

func TestConfig_ValidateAPI(t *testing.T) {
	cfg := &Config{IsProduction: true, AuthBaseURL: "http://api", AuthUIBaseURL: "https://ui", AppleClientID: "apple"}
	err := cfg.ValidateAPI()
	require.Error(t, err)
	assert.ErrorContains(t, err, "MINI_MOVIE_UI_SECRET")
	assert.ErrorContains(t, err, "SESSION_SECRET")
	assert.ErrorContains(t, err, "TOKEN_ENCRYPTION_KEY")
	assert.ErrorContains(t, err, "AUTH_BASE_URL must use HTTPS")
	assert.NotContains(t, err.Error(), "OAuth provider")

	cfg.MiniMovieUiSecret = "s"
	cfg.SessionSecret = make([]byte, 32)
	cfg.TokenEncryptionKey = make([]byte, 32)
	cfg.AuthBaseURL = "https://api"
	assert.NoError(t, cfg.ValidateAPI())
}
