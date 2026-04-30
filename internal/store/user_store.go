package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type User struct {
	ID             string     `json:"id"`
	Username       *string    `json:"username"`
	GivenName      *string    `json:"givenName,omitempty"`
	AvatarURL      *string    `json:"avatarUrl,omitempty"`
	LastExportedAt *time.Time `json:"-"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type UserRepository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	ListOAuthAccounts(ctx context.Context, userID string) ([]OAuthAccount, error)
	CanExportToday(ctx context.Context, id string) (bool, error)
	UpsertFromOAuth(ctx context.Context, provider, providerID, encryptedRefreshToken string, avatarURL, givenName *string) (*UpsertResult, error)
	MarkExported(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

func (s *UserStore) GetByID(ctx context.Context, id string) (*User, error) {
	defer metrics.TrackDbDuration(ctx, "users.get_by_id")()
	query := `select id, username, given_name, avatar_url, last_exported_at, created_at, updated_at from users where id = $1`
	var u User
	err := s.pool.QueryRow(ctx, query, id).Scan(&u.ID, &u.Username, &u.GivenName, &u.AvatarURL, &u.LastExportedAt, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

type OAuthAccount struct {
	ID                    string
	UserID                string
	Provider              string
	ProviderID            string
	EncryptedRefreshToken *string
}

func (s *UserStore) ListOAuthAccounts(ctx context.Context, userID string) ([]OAuthAccount, error) {
	defer metrics.TrackDbDuration(ctx, "users.list_oauth_accounts")()
	query := `select id, user_id, provider, provider_id, encrypted_refresh_token
	          from oauth_accounts where user_id = $1`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []OAuthAccount
	for rows.Next() {
		var a OAuthAccount
		if err := rows.Scan(&a.ID, &a.UserID, &a.Provider, &a.ProviderID, &a.EncryptedRefreshToken); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *UserStore) CanExportToday(ctx context.Context, id string) (bool, error) {
	defer metrics.TrackDbDuration(ctx, "users.can_export_today")()
	var lastExported *time.Time
	err := s.pool.QueryRow(ctx,
		`select last_exported_at from users where id = $1`, id,
	).Scan(&lastExported)
	if err == pgx.ErrNoRows || lastExported == nil {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return time.Since(*lastExported) >= 24*time.Hour, nil
}

type UpsertResult struct {
	User      *User
	IsNewUser bool
}

func (s *UserStore) UpsertFromOAuth(ctx context.Context, provider, providerID, encryptedRefreshToken string, avatarURL, givenName *string) (*UpsertResult, error) {
	defer metrics.TrackDbDuration(ctx, "users.upsert_from_oauth")()

	var result *UpsertResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var existingUserID string
		err := tx.QueryRow(ctx,
			`select user_id from oauth_accounts where provider = $1 and provider_id = $2`,
			provider, providerID,
		).Scan(&existingUserID)

		if err == nil {
			if encryptedRefreshToken != "" {
				if _, err := tx.Exec(ctx,
					`update oauth_accounts set encrypted_refresh_token = $1, updated_at = now() where provider = $2 and provider_id = $3`,
					encryptedRefreshToken, provider, providerID,
				); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx,
				`update users set
					given_name = coalesce(nullif($1, ''), given_name),
					avatar_url = coalesce(nullif($2, ''), avatar_url),
					updated_at = now()
				where id = $3`,
				ptrToEmpty(givenName), ptrToEmpty(avatarURL), existingUserID,
			); err != nil {
				return err
			}

			var u User
			if err := tx.QueryRow(ctx, `select id, username, given_name, avatar_url, last_exported_at, created_at, updated_at from users where id = $1`, existingUserID).
				Scan(&u.ID, &u.Username, &u.GivenName, &u.AvatarURL, &u.LastExportedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return err
			}

			result = &UpsertResult{User: &u, IsNewUser: false}
			return nil
		}

		if err != pgx.ErrNoRows {
			return err
		}

		userID := uuid.New().String()
		oauthID := uuid.New().String()

		if _, err := tx.Exec(ctx,
			`insert into users (id, given_name, avatar_url, created_at, updated_at) values ($1, $2, $3, now(), now())`,
			userID, givenName, avatarURL,
		); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`insert into oauth_accounts (id, user_id, provider, provider_id, encrypted_refresh_token, created_at, updated_at)
			 values ($1, $2, $3, $4, $5, now(), now())`,
			oauthID, userID, provider, providerID, nilIfEmpty(encryptedRefreshToken),
		); err != nil {
			return err
		}

		result = &UpsertResult{
			User:      &User{ID: userID, GivenName: givenName, AvatarURL: avatarURL, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			IsNewUser: true,
		}
		return nil
	})

	return result, err
}

func (s *UserStore) MarkExported(ctx context.Context, id string) error {
	defer metrics.TrackDbDuration(ctx, "users.mark_exported")()
	_, err := s.pool.Exec(ctx,
		`update users set last_exported_at = now() where id = $1`, id,
	)
	return err
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	defer metrics.TrackDbDuration(ctx, "users.delete")()
	_, err := s.pool.Exec(ctx, `delete from users where id = $1`, id)
	return err
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrToEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
