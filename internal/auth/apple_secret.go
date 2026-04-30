package auth

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const appleSecretMaxLifetime = 180 * 24 * time.Hour // 6 months

func GenerateAppleClientSecret(teamID, keyID, clientID string, privateKeyPEM []byte) (string, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return "", errors.New("invalid Apple private key PEM")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parsing Apple private key: %w", err)
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": teamID,
		"iat": now.Unix(),
		"exp": now.Add(appleSecretMaxLifetime).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": clientID,
	})
	token.Header["kid"] = keyID

	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("signing Apple client secret: %w", err)
	}

	return signed, nil
}
