package store

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

const personColumns = `id, source_id, slug, name, date_of_birth, date_of_death, profile_path, known_for_department,
	also_known_as, popularity, payload, insights, insights_at, stale, fetched_at, created_at, updated_at`

type PersonDates struct {
	ID          int
	DateOfBirth string
	DateOfDeath string
	Popularity  float64
	Fetched     bool
}

type Person struct {
	ID                 int             `db:"id"`
	SourceID           int             `db:"source_id"`
	Slug               string          `db:"slug"`
	Name               string          `db:"name"`
	DateOfBirth        *time.Time      `db:"date_of_birth"`
	DateOfDeath        *time.Time      `db:"date_of_death"`
	ProfilePath        *string         `db:"profile_path"`
	KnownForDepartment *string         `db:"known_for_department"`
	AlsoKnownAs        []string        `db:"also_known_as"`
	Popularity         float64         `db:"popularity"`
	Payload            json.RawMessage `db:"payload"`
	Insights           json.RawMessage `db:"insights"`
	InsightsAt         *time.Time      `db:"insights_at"`
	Stale              bool            `db:"stale"`
	FetchedAt          *time.Time      `db:"fetched_at"`
	CreatedAt          time.Time       `db:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at"`
}

// PersonSkeleton is what a credit list or an export file knows about a person.
type PersonSkeleton struct {
	SourceID           int
	Name               string
	ProfilePath        *string
	KnownForDepartment *string
	Popularity         float64
}

type PersonStore struct {
	catalogTable
}

func NewPersonStore(pool *pgxpool.Pool) *PersonStore {
	return &PersonStore{catalogTable{pool: pool, name: "people"}}
}

func (s *PersonStore) GetDates(ctx context.Context, sourceIDs []int) (map[int]PersonDates, error) {
	result := make(map[int]PersonDates, len(sourceIDs))
	if len(sourceIDs) == 0 {
		return result, nil
	}
	defer metrics.TrackDbDuration(ctx, "read")()

	rows, err := s.pool.Query(ctx, `
		select id, source_id, date_of_birth, date_of_death, popularity, payload is not null
		from people where source_id = any($1)`, sourceIDs)
	if err != nil {
		return nil, fmt.Errorf("person store: get dates: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sourceID int
		var dob, dod pgtype.Date
		var d PersonDates
		if err := rows.Scan(&d.ID, &sourceID, &dob, &dod, &d.Popularity, &d.Fetched); err != nil {
			return nil, fmt.Errorf("person store: get dates: %w", err)
		}
		if dob.Valid {
			d.DateOfBirth = dob.Time.Format(time.DateOnly)
		}
		if dod.Valid {
			d.DateOfDeath = dod.Time.Format(time.DateOnly)
		}
		result[sourceID] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("person store: get dates: %w", err)
	}
	return result, nil
}

func (s *PersonStore) GetByID(ctx context.Context, id int) (*Person, error) {
	return s.getOne(ctx, `where id = $1`, id)
}

func (s *PersonStore) GetBySourceID(ctx context.Context, sourceID int) (*Person, error) {
	return s.getOne(ctx, `where source_id = $1`, sourceID)
}

// UpsertHydrated writes a fully populated people row and returns the row's id.
func (s *PersonStore) UpsertHydrated(ctx context.Context, db DBTX, p Person) (int, error) {
	defer metrics.TrackDbDuration(ctx, "write")()
	if p.AlsoKnownAs == nil {
		p.AlsoKnownAs = []string{}
	}

	id, err := scanID(db.QueryRow(ctx, `
		insert into people (source_id, name, date_of_birth, date_of_death, profile_path, known_for_department,
		                    also_known_as, popularity, payload, stale, fetched_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, now(), now())
		on conflict (source_id) do update set
			name                 = excluded.name,
			date_of_birth        = excluded.date_of_birth,
			date_of_death        = excluded.date_of_death,
			profile_path         = excluded.profile_path,
			known_for_department = excluded.known_for_department,
			also_known_as        = excluded.also_known_as,
			popularity           = excluded.popularity,
			payload              = excluded.payload,
			stale                = false,
			fetched_at           = now(),
			updated_at           = now()
		returning id`,
		p.SourceID, p.Name, nullTime(p.DateOfBirth), nullTime(p.DateOfDeath), p.ProfilePath, p.KnownForDepartment,
		p.AlsoKnownAs, p.Popularity, p.Payload))
	if err != nil {
		return 0, fmt.Errorf("person store: upsert source %d: %w", p.SourceID, err)
	}
	return id, nil
}

// UpsertSkeleton writes rows from an export file or a credit list.
func (s *PersonStore) UpsertSkeleton(ctx context.Context, db DBTX, rows []PersonSkeleton) error {
	if len(rows) == 0 {
		return nil
	}
	defer metrics.TrackDbDuration(ctx, "write")()

	// Same order in every statement, so concurrent bulk upserts cannot deadlock each other.
	slices.SortFunc(rows, func(a, b PersonSkeleton) int { return cmp.Compare(a.SourceID, b.SourceID) })
	sourceIDs := make([]int32, len(rows))
	names := make([]string, len(rows))
	profiles := make([]*string, len(rows))
	departments := make([]*string, len(rows))
	popularity := make([]float32, len(rows))
	for i, r := range rows {
		sourceIDs[i] = int32(r.SourceID)
		names[i] = r.Name
		profiles[i] = r.ProfilePath
		departments[i] = r.KnownForDepartment
		popularity[i] = float32(r.Popularity)
	}

	_, err := db.Exec(ctx, `
		insert into people (source_id, name, profile_path, known_for_department, popularity, updated_at)
		select u.source_id, u.name, u.profile, u.department, u.popularity, now()
		from unnest($1::int[], $2::text[], $3::text[], $4::text[], $5::real[]) as u(source_id, name, profile, department, popularity)
		on conflict (source_id) do update set
			popularity           = case when people.payload is null and excluded.popularity > 0
			                       then excluded.popularity else people.popularity end,
			name                 = case when people.payload is null then excluded.name else people.name end,
			profile_path         = case when people.payload is null then coalesce(excluded.profile_path, people.profile_path) else people.profile_path end,
			known_for_department = case when people.payload is null then coalesce(excluded.known_for_department, people.known_for_department) else people.known_for_department end,
			updated_at           = now()`,
		sourceIDs, names, profiles, departments, popularity)
	if err != nil {
		return fmt.Errorf("person store: upsert skeletons: %w", err)
	}
	return nil
}

// GetInsights returns the cached person insights result and when it was produced.
func (s *PersonStore) GetInsights(ctx context.Context, id int) (json.RawMessage, *time.Time, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	var data json.RawMessage
	var at *time.Time
	err := s.pool.QueryRow(ctx, `select insights, insights_at from people where id = $1`, id).Scan(&data, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("person store: get insights %d: %w", id, err)
	}
	return data, at, nil
}

func (s *PersonStore) SetInsights(ctx context.Context, id int, data json.RawMessage) error {
	defer metrics.TrackDbDuration(ctx, "write")()

	_, err := s.pool.Exec(ctx, `update people set insights = $2, insights_at = now(), updated_at = now() where id = $1`, id, data)
	if err != nil {
		return fmt.Errorf("person store: set insights %d: %w", id, err)
	}
	return nil
}

func (s *PersonStore) GetPeople(ctx context.Context, sourceIDs []int) (map[int]PersonDates, error) {
	return s.GetDates(ctx, sourceIDs)
}

func (s *PersonStore) UpsertPersonBatch(ctx context.Context, people map[int]PersonDates, names map[int]string) error {
	if len(people) == 0 {
		return nil
	}
	defer metrics.TrackDbDuration(ctx, "write")()

	sourceIDs := make([]int32, 0, len(people))
	nameArr := make([]string, 0, len(people))
	dobs := make([]*string, 0, len(people))
	dods := make([]*string, 0, len(people))
	for sourceID, dates := range people {
		sourceIDs = append(sourceIDs, int32(sourceID))
		nameArr = append(nameArr, names[sourceID])
		dob, dod := dates.DateOfBirth, dates.DateOfDeath
		dobs = append(dobs, emptyToNil(&dob))
		dods = append(dods, emptyToNil(&dod))
	}

	_, err := s.pool.Exec(ctx, `
		insert into people (source_id, name, date_of_birth, date_of_death, updated_at)
		select u.source_id, u.name, u.dob::date, u.dod::date, now()
		from unnest($1::int[], $2::text[], $3::text[], $4::text[]) as u(source_id, name, dob, dod)
		on conflict (source_id) do update set
			name          = case when people.payload is null then excluded.name else people.name end,
			date_of_birth = coalesce(excluded.date_of_birth, people.date_of_birth),
			date_of_death = coalesce(excluded.date_of_death, people.date_of_death),
			updated_at    = now()`, sourceIDs, nameArr, dobs, dods)
	if err != nil {
		return fmt.Errorf("person store: upsert batch: %w", err)
	}
	return nil
}

func (s *PersonStore) MarkPeopleStale(ctx context.Context, sourceIDs []int) (int64, error) {
	return s.MarkStale(ctx, sourceIDs)
}

func (s *PersonStore) getOne(ctx context.Context, where string, arg any) (*Person, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	rows, err := s.pool.Query(ctx, `select `+personColumns+` from people `+where, arg)
	if err != nil {
		return nil, fmt.Errorf("person store: get: %w", err)
	}
	p, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Person])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("person store: get: %w", err)
	}
	return p, nil
}
