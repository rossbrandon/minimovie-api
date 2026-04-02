package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

type PersonDates struct {
	DateOfBirth string
	DateOfDeath string
	Popularity  float64
	Fetched     bool
}

type PersonStore struct {
	pool *pgxpool.Pool
}

func NewPersonStore(pool *pgxpool.Pool) *PersonStore {
	return &PersonStore{pool: pool}
}

func (s *PersonStore) GetPeople(ctx context.Context, personIDs []int) (map[int]PersonDates, error) {
	if len(personIDs) == 0 {
		return make(map[int]PersonDates), nil
	}

	defer metrics.TrackDbDuration(ctx, "read")()

	query := `
		select id, date_of_birth, date_of_death, popularity, fetched
		from people
		where id = any($1)
	`

	rows, err := s.pool.Query(ctx, query, personIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]PersonDates)
	for rows.Next() {
		var id int
		var dob, dod pgtype.Date
		var popularity float64
		var fetched bool

		if err := rows.Scan(&id, &dob, &dod, &popularity, &fetched); err != nil {
			return nil, err
		}

		dates := PersonDates{Fetched: fetched, Popularity: popularity}
		if dob.Valid {
			dates.DateOfBirth = dob.Time.Format(time.DateOnly)
		}
		if dod.Valid {
			dates.DateOfDeath = dod.Time.Format(time.DateOnly)
		}
		result[id] = dates
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *PersonStore) UpsertPersonBatch(ctx context.Context, people map[int]PersonDates, names map[int]string) error {
	if len(people) == 0 {
		return nil
	}

	defer metrics.TrackDbDuration(ctx, "write")()

	batch := &pgx.Batch{}
	query := `
		insert into people (id, name, date_of_birth, date_of_death, fetched, updated_at)
		values ($1, $2, $3, $4, $5, now())
		on conflict (id) do update set
			name = coalesce(excluded.name, people.name),
			date_of_birth = excluded.date_of_birth,
			date_of_death = excluded.date_of_death,
			fetched = excluded.fetched,
			updated_at = now()
	`

	for id, dates := range people {
		var dobPtr, dodPtr *string
		if dates.DateOfBirth != "" {
			dobPtr = &dates.DateOfBirth
		}
		if dates.DateOfDeath != "" {
			dodPtr = &dates.DateOfDeath
		}

		name := names[id]
		batch.Queue(query, id, name, dobPtr, dodPtr, dates.Fetched)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()

	for range people {
		if _, err := results.Exec(); err != nil {
			log.Error().Err(err).Msg("failed to execute batch upsert to people table in database")
			return err
		}
	}

	return nil
}

func (s *PersonStore) MarkPeopleStale(ctx context.Context, personIDs []int) (int64, error) {
	if len(personIDs) == 0 {
		return 0, nil
	}

	defer metrics.TrackDbDuration(ctx, "write")()

	query := `
		update people
		set fetched = false, updated_at = now()
		where id = any($1) and fetched = true
	`

	result, err := s.pool.Exec(ctx, query, personIDs)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

func (s *PersonStore) Close() {
	s.pool.Close()
}
