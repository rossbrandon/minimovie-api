package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserStore_UpsertFromOAuth_NewUser(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	result, err := s.UpsertFromOAuth(ctx, "google", "provider-123", "", nil, nil)
	require.NoError(t, err)
	assert.True(t, result.IsNewUser)
	assert.NotEmpty(t, result.User.ID)
}

func TestUserStore_UpsertFromOAuth_ExistingUser(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	first, err := s.UpsertFromOAuth(ctx, "google", "provider-456", "", nil, nil)
	require.NoError(t, err)

	second, err := s.UpsertFromOAuth(ctx, "google", "provider-456", "", nil, nil)
	require.NoError(t, err)
	assert.False(t, second.IsNewUser)
	assert.Equal(t, first.User.ID, second.User.ID)
}

func TestUserStore_UpsertFromOAuth_UpdatesAvatar(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	_, err := s.UpsertFromOAuth(ctx, "google", "provider-789", "", nil, nil)
	require.NoError(t, err)

	avatar := "https://example.com/avatar.png"
	result, err := s.UpsertFromOAuth(ctx, "google", "provider-789", "", &avatar, nil)
	require.NoError(t, err)
	require.NotNil(t, result.User.AvatarURL)
	assert.Equal(t, avatar, *result.User.AvatarURL)
}

func TestUserStore_UpsertFromOAuth_UpdatesRefreshToken(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	result, err := s.UpsertFromOAuth(ctx, "google", "provider-rt", "", nil, nil)
	require.NoError(t, err)

	_, err = s.UpsertFromOAuth(ctx, "google", "provider-rt", "encrypted-refresh-token", nil, nil)
	require.NoError(t, err)

	accounts, err := s.ListOAuthAccounts(ctx, result.User.ID)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.NotNil(t, accounts[0].EncryptedRefreshToken)
	assert.Equal(t, "encrypted-refresh-token", *accounts[0].EncryptedRefreshToken)
}

func TestUserStore_GetByID(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	userID := createTestUser(t)

	user, err := s.GetByID(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, userID, user.ID)

	missing, err := s.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestUserStore_ListOAuthAccounts(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	result, err := s.UpsertFromOAuth(ctx, "google", "multi-1", "", nil, nil)
	require.NoError(t, err)
	userID := result.User.ID

	_, err = testPool.Exec(ctx,
		`INSERT INTO oauth_accounts (id, user_id, provider, provider_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, 'apple', 'multi-2', now(), now())`,
		userID,
	)
	require.NoError(t, err)

	accounts, err := s.ListOAuthAccounts(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, accounts, 2)

	providers := map[string]bool{}
	for _, a := range accounts {
		providers[a.Provider] = true
	}
	assert.True(t, providers["google"])
	assert.True(t, providers["apple"])
}

func TestUserStore_CanExportToday_NoExport(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	userID := createTestUser(t)

	can, err := s.CanExportToday(ctx, userID)
	require.NoError(t, err)
	assert.True(t, can)
}

func TestUserStore_MarkExported_ThenCanExport(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	userID := createTestUser(t)

	err := s.MarkExported(ctx, userID)
	require.NoError(t, err)

	can, err := s.CanExportToday(ctx, userID)
	require.NoError(t, err)
	assert.False(t, can)
}

func TestUserStore_Delete(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewUserStore(testPool)

	userID := createTestUser(t)

	err := s.Delete(ctx, userID)
	require.NoError(t, err)

	user, err := s.GetByID(ctx, userID)
	require.NoError(t, err)
	assert.Nil(t, user)
}
