package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
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
	if metrics.M != nil {
		metrics.M.RecordPeopleUpsertBatchSize(ctx, len(people))
	}

	ids := make([]int32, 0, len(people))
	nameArr := make([]string, 0, len(people))
	dobs := make([]*string, 0, len(people))
	dods := make([]*string, 0, len(people))
	fetchedArr := make([]bool, 0, len(people))

	for id, dates := range people {
		ids = append(ids, int32(id))
		nameArr = append(nameArr, names[id])

		var dobPtr, dodPtr *string
		if dates.DateOfBirth != "" {
			dob := dates.DateOfBirth
			dobPtr = &dob
		}
		if dates.DateOfDeath != "" {
			dod := dates.DateOfDeath
			dodPtr = &dod
		}
		dobs = append(dobs, dobPtr)
		dods = append(dods, dodPtr)
		fetchedArr = append(fetchedArr, dates.Fetched)
	}

	query := `
		insert into people (id, name, date_of_birth, date_of_death, fetched, updated_at)
		select u.id, u.name, u.dob::date, u.dod::date, u.fetched, now()
		from unnest($1::int[], $2::text[], $3::text[], $4::text[], $5::bool[]) as u(id, name, dob, dod, fetched)
		on conflict (id) do update set
			name = coalesce(excluded.name, people.name),
			date_of_birth = excluded.date_of_birth,
			date_of_death = excluded.date_of_death,
			fetched = excluded.fetched,
			updated_at = now()
	`

	_, err := s.pool.Exec(ctx, query, ids, nameArr, dobs, dods, fetchedArr)
	return err
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
