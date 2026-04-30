package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type Session struct {
	ID        string    `json:"id"`
	TokenHash string    `json:"-"`
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type SessionWithUser struct {
	Session
	User User
}

type SessionRepository interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (*SessionWithUser, error)
	Create(ctx context.Context, tokenHash, userID string) (*Session, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

type SessionStore struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

func (s *SessionStore) GetByTokenHash(ctx context.Context, tokenHash string) (*SessionWithUser, error) {
	defer metrics.TrackDbDuration(ctx, "sessions.get_by_token_hash")()
	query := `
		select s.id, s.token_hash, s.user_id, s.expires_at, s.created_at,
		       u.id, u.username, u.given_name, u.avatar_url, u.created_at, u.updated_at
		from sessions s
		join users u on u.id = s.user_id
		where s.token_hash = $1 and s.expires_at > now()
	`
	var sw SessionWithUser
	err := s.pool.QueryRow(ctx, query, tokenHash).Scan(
		&sw.Session.ID, &sw.Session.TokenHash, &sw.Session.UserID,
		&sw.Session.ExpiresAt, &sw.Session.CreatedAt,
		&sw.User.ID, &sw.User.Username, &sw.User.GivenName, &sw.User.AvatarURL, &sw.User.CreatedAt, &sw.User.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sw, nil
}

func (s *SessionStore) Create(ctx context.Context, tokenHash, userID string) (*Session, error) {
	defer metrics.TrackDbDuration(ctx, "sessions.create")()
	sess := &Session{
		ID:        uuid.New().String(),
		TokenHash: tokenHash,
		UserID:    userID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	_, err := s.pool.Exec(ctx,
		`insert into sessions (id, token_hash, user_id, expires_at, created_at)
		 values ($1, $2, $3, $4, $5)`,
		sess.ID, sess.TokenHash, sess.UserID, sess.ExpiresAt, sess.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *SessionStore) Delete(ctx context.Context, tokenHash string) error {
	defer metrics.TrackDbDuration(ctx, "sessions.delete")()
	_, err := s.pool.Exec(ctx, `delete from sessions where token_hash = $1`, tokenHash)
	return err
}

func (s *SessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	defer metrics.TrackDbDuration(ctx, "sessions.delete_by_user_id")()
	_, err := s.pool.Exec(ctx, `delete from sessions where user_id = $1`, userID)
	return err
}

func (s *SessionStore) DeleteExpired(ctx context.Context) (int64, error) {
	defer metrics.TrackDbDuration(ctx, "sessions.delete_expired")()
	tag, err := s.pool.Exec(ctx, `delete from sessions where expires_at < now()`)
	if err != nil {
		return 0, err
	}
	count := tag.RowsAffected()
	if metrics.M != nil {
		metrics.M.RecordDbPurge(ctx, "sessions", count)
	}
	return count, nil
}

func (s *SessionStore) TableName() string { return "sessions" }
