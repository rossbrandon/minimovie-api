package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestP256PEM(t *testing.T) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return pemBytes, key
}

func TestGenerateAppleClientSecret(t *testing.T) {
	pemKey, privKey := generateTestP256PEM(t)

	tests := []struct {
		name       string
		teamID     string
		keyID      string
		clientID   string
		keyPEM     []byte
		wantErr    bool
		checkToken bool
	}{
		{
			name:       "valid key produces parseable JWT",
			teamID:     "TEAM123",
			keyID:      "KEY456",
			clientID:   "com.example.app",
			keyPEM:     pemKey,
			checkToken: true,
		},
		{
			name:     "invalid PEM returns error",
			teamID:   "TEAM123",
			keyID:    "KEY456",
			clientID: "com.example.app",
			keyPEM:   []byte("not-a-pem"),
			wantErr:  true,
		},
		{
			name:     "garbage PEM block returns error",
			teamID:   "TEAM123",
			keyID:    "KEY456",
			clientID: "com.example.app",
			keyPEM: pem.EncodeToMemory(&pem.Block{
				Type:  "PRIVATE KEY",
				Bytes: []byte("garbage"),
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret, err := GenerateAppleClientSecret(tt.teamID, tt.keyID, tt.clientID, tt.keyPEM)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, secret)

			if !tt.checkToken {
				return
			}

			parsed, err := jwt.Parse(secret, func(token *jwt.Token) (interface{}, error) {
				return &privKey.PublicKey, nil
			})
			require.NoError(t, err)
			require.True(t, parsed.Valid)

			claims, ok := parsed.Claims.(jwt.MapClaims)
			require.True(t, ok)
			assert.Equal(t, tt.teamID, claims["iss"])
			assert.Equal(t, tt.clientID, claims["sub"])
			assert.Equal(t, "https://appleid.apple.com", claims["aud"])

			assert.Equal(t, tt.keyID, parsed.Header["kid"])
		})
	}
}
