package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type AuthCode struct {
	ID              string
	TokenHash       string
	UserID          string
	RawSessionToken string
	ExpiresAt       time.Time
}

type AuthCodeRepository interface {
	Create(ctx context.Context, tokenHash, userID, rawSessionToken string) (string, error)
	Exchange(ctx context.Context, id string) (*AuthCode, error)
}

type AuthCodeStore struct {
	pool          *pgxpool.Pool
	encryptionKey []byte
}

func NewAuthCodeStore(pool *pgxpool.Pool, encryptionKey []byte) *AuthCodeStore {
	return &AuthCodeStore{pool: pool, encryptionKey: encryptionKey}
}

func (s *AuthCodeStore) Create(ctx context.Context, tokenHash, userID, rawSessionToken string) (string, error) {
	defer metrics.TrackDbDuration(ctx, "auth_code.create")()
	id, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}

	encryptedToken, err := auth.Encrypt([]byte(rawSessionToken), s.encryptionKey)
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(60 * time.Second)
	_, err = s.pool.Exec(ctx,
		`insert into auth_code (id, token_hash, encrypted_session_token, user_id, expires_at)
		 values ($1, $2, $3, $4, $5)`,
		id, tokenHash, encryptedToken, userID, expiresAt,
	)
	if err != nil {
		return "", err
	}

	return id, nil
}

func (s *AuthCodeStore) Exchange(ctx context.Context, id string) (*AuthCode, error) {
	defer metrics.TrackDbDuration(ctx, "auth_code.exchange")()
	query := `delete from auth_code where id = $1 and expires_at > now()
	          returning id, token_hash, encrypted_session_token, user_id, expires_at`
	var ac AuthCode
	var encryptedToken string
	err := s.pool.QueryRow(ctx, query, id).Scan(&ac.ID, &ac.TokenHash, &encryptedToken, &ac.UserID, &ac.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rawBytes, err := auth.Decrypt(encryptedToken, s.encryptionKey)
	if err != nil {
		return nil, err
	}
	ac.RawSessionToken = string(rawBytes)

	return &ac, nil
}

func (s *AuthCodeStore) DeleteExpired(ctx context.Context) (int64, error) {
	defer metrics.TrackDbDuration(ctx, "auth_code.delete_expired")()
	tag, err := s.pool.Exec(ctx, `delete from auth_code where expires_at < now()`)
	if err != nil {
		return 0, err
	}
	count := tag.RowsAffected()
	if metrics.M != nil {
		metrics.M.RecordDbPurge(ctx, "auth_code", count)
	}
	return count, nil
}

func (s *AuthCodeStore) TableName() string { return "auth_code" }
