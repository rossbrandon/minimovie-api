package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type UserAchievement struct {
	ID                  string     `json:"id"`
	UserID              string     `json:"-"`
	AchievementID       string     `json:"achievementId"`
	EarnedViaMediaType  *string    `json:"earnedViaMediaType,omitempty"`
	EarnedViaMediaID    *int       `json:"earnedViaMediaId,omitempty"`
	EarnedViaMediaTitle *string    `json:"earnedViaMediaTitle,omitempty"`
	SeenAt              *time.Time `json:"seenAt,omitempty"`
	EarnedAt            time.Time  `json:"earnedAt"`
}

type AchievementRepository interface {
	ListByUserID(ctx context.Context, userID string) ([]UserAchievement, error)
	ListUnseen(ctx context.Context, userID string) ([]UserAchievement, error)
	CountUnseen(ctx context.Context, userID string) (int, error)
	MarkAllSeen(ctx context.Context, userID string) error
}

type AchievementStore struct {
	pool *pgxpool.Pool
}

func NewAchievementStore(pool *pgxpool.Pool) *AchievementStore {
	return &AchievementStore{pool: pool}
}

func (s *AchievementStore) ListByUserID(ctx context.Context, userID string) ([]UserAchievement, error) {
	defer metrics.TrackDbDuration(ctx, "achievements.list_by_user_id")()
	query := `select id, achievement_id, earned_via_media_type, earned_via_media_id, earned_via_media_title, seen_at, earned_at
	          from user_achievement where user_id = $1 order by earned_at desc`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var achievements []UserAchievement
	for rows.Next() {
		var a UserAchievement
		if err := rows.Scan(&a.ID, &a.AchievementID, &a.EarnedViaMediaType, &a.EarnedViaMediaID, &a.EarnedViaMediaTitle, &a.SeenAt, &a.EarnedAt); err != nil {
			return nil, err
		}
		achievements = append(achievements, a)
	}
	return achievements, rows.Err()
}

func (s *AchievementStore) ListUnseen(ctx context.Context, userID string) ([]UserAchievement, error) {
	defer metrics.TrackDbDuration(ctx, "achievements.list_unseen")()
	query := `select id, achievement_id, earned_via_media_type, earned_via_media_id, earned_via_media_title, seen_at, earned_at
	          from user_achievement where user_id = $1 and seen_at is null order by earned_at desc`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var achievements []UserAchievement
	for rows.Next() {
		var a UserAchievement
		if err := rows.Scan(&a.ID, &a.AchievementID, &a.EarnedViaMediaType, &a.EarnedViaMediaID, &a.EarnedViaMediaTitle, &a.SeenAt, &a.EarnedAt); err != nil {
			return nil, err
		}
		achievements = append(achievements, a)
	}
	return achievements, rows.Err()
}

func (s *AchievementStore) Exists(ctx context.Context, userID, achievementID string) (bool, error) {
	defer metrics.TrackDbDuration(ctx, "achievements.exists")()
	var exists bool
	err := s.pool.QueryRow(ctx,
		`select exists(select 1 from user_achievement where user_id = $1 and achievement_id = $2)`,
		userID, achievementID,
	).Scan(&exists)
	return exists, err
}

func (s *AchievementStore) CountUnseen(ctx context.Context, userID string) (int, error) {
	defer metrics.TrackDbDuration(ctx, "achievements.count_unseen")()
	var count int
	err := s.pool.QueryRow(ctx,
		`select count(*) from user_achievement where user_id = $1 and seen_at is null`,
		userID,
	).Scan(&count)
	return count, err
}

func (s *AchievementStore) Create(ctx context.Context, userID, achievementID, mediaType string, mediaID int, mediaTitle string) error {
	defer metrics.TrackDbDuration(ctx, "achievements.create")()
	_, err := s.pool.Exec(ctx,
		`insert into user_achievement (user_id, achievement_id, earned_via_media_type, earned_via_media_id, earned_via_media_title)
		 values ($1, $2, $3, $4, $5)
		 on conflict (user_id, achievement_id) do nothing`,
		userID, achievementID, mediaType, mediaID, mediaTitle,
	)
	return err
}

func (s *AchievementStore) MarkAllSeen(ctx context.Context, userID string) error {
	defer metrics.TrackDbDuration(ctx, "achievements.mark_all_seen")()
	_, err := s.pool.Exec(ctx,
		`update user_achievement set seen_at = now() where user_id = $1 and seen_at is null`,
		userID,
	)
	return err
}
