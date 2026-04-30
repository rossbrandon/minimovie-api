package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken(t *testing.T) {
	t.Run("produces non-empty token", func(t *testing.T) {
		token, err := GenerateToken()
		require.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("two calls produce different tokens", func(t *testing.T) {
		t1, err := GenerateToken()
		require.NoError(t, err)
		t2, err := GenerateToken()
		require.NoError(t, err)
		assert.NotEqual(t, t1, t2)
	})
}

func TestHashToken(t *testing.T) {
	secret := []byte("test-secret-key")

	t.Run("deterministic", func(t *testing.T) {
		h1 := HashToken("my-token", secret)
		h2 := HashToken("my-token", secret)
		assert.Equal(t, h1, h2)
	})

	t.Run("different inputs produce different outputs", func(t *testing.T) {
		h1 := HashToken("token-a", secret)
		h2 := HashToken("token-b", secret)
		assert.NotEqual(t, h1, h2)
	})
}

func TestEncryptDecrypt(t *testing.T) {
	validKey := []byte("01234567890123456789012345678901") // 32 bytes

	t.Run("roundtrip", func(t *testing.T) {
		plaintext := []byte("hello, world")
		ct, err := Encrypt(plaintext, validKey)
		require.NoError(t, err)

		got, err := Decrypt(ct, validKey)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got)
	})

	t.Run("decrypt with wrong key fails", func(t *testing.T) {
		plaintext := []byte("secret data")
		ct, err := Encrypt(plaintext, validKey)
		require.NoError(t, err)

		wrongKey := []byte("99999999999999999999999999999999")
		_, err = Decrypt(ct, wrongKey)
		assert.Error(t, err)
	})

	t.Run("encrypt with too-short key fails", func(t *testing.T) {
		_, err := Encrypt([]byte("data"), []byte("short"))
		assert.Error(t, err)
	})
}
