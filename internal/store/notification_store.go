package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type NotificationSeenRepository interface {
	MarkSeen(ctx context.Context, jti, provider string) (bool, error)
}

type NotificationSeenStore struct {
	pool *pgxpool.Pool
}

func NewNotificationSeenStore(pool *pgxpool.Pool) *NotificationSeenStore {
	return &NotificationSeenStore{pool: pool}
}

func (s *NotificationSeenStore) MarkSeen(ctx context.Context, jti, provider string) (bool, error) {
	defer metrics.TrackDbDuration(ctx, "notifications.mark_seen")()
	tag, err := s.pool.Exec(ctx,
		`insert into provider_notifications_seen (jti, provider) values ($1, $2) on conflict (jti) do nothing`,
		jti, provider,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *NotificationSeenStore) DeleteExpired(ctx context.Context) (int64, error) {
	defer metrics.TrackDbDuration(ctx, "notifications.delete_expired")()
	tag, err := s.pool.Exec(ctx,
		`delete from provider_notifications_seen where received_at < now() - interval '30 days'`,
	)
	if err != nil {
		return 0, err
	}
	count := tag.RowsAffected()
	if metrics.M != nil {
		metrics.M.RecordDbPurge(ctx, "provider_notifications_seen", count)
	}
	return count, nil
}

func (s *NotificationSeenStore) TableName() string { return "provider_notifications_seen" }
