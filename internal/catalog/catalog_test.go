package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSlugID(t *testing.T) {
	cases := map[string]int{
		"155-the-dark-knight": 155,
		"155":                 155,
		"7-":                  7,
		"the-dark-knight-155": 0,
		"":                    0,
		"0-nothing":           0,
		"12abc":               0,
	}
	for slug, want := range cases {
		id, ok := ParseSlugID(slug)
		assert.Equal(t, want != 0, ok, slug)
		assert.Equal(t, want, id, slug)
	}
}

func TestParseEntity(t *testing.T) {
	for _, name := range []string{"movies", "series", "seasons", "episodes", "people"} {
		e, err := ParseEntity(name)
		require.NoError(t, err)
		assert.Equal(t, name, e.String())
	}
	_, err := ParseEntity("movie")
	assert.Error(t, err, "entities are spelled like their tables")
}

func TestHydrate_MoviesAndTheirPeople(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	f.movies[550] = fightClub()
	f.movies[551] = tmdb.Movie{ID: 551, Title: "Second", ReleaseDate: "2000-01-01", Popularity: 5}
	f.people[819] = person(819, "Edward Norton", "1969-08-18")
	f.people[287] = person(287, "Brad Pitt", "1963-12-18")
	f.people[7467] = person(7467, "David Fincher", "1962-08-28")
	f.people[7474] = person(7474, "Art Linson", "")
	svc := newTestService(t, f, 10)
	movies := store.NewMovieStore(testPool)
	people := store.NewPersonStore(testPool)

	require.NoError(t, movies.UpsertSkeleton(ctx, testPool, []store.Skeleton{
		{SourceID: 550, Title: "Fight Club", Popularity: 61}, {SourceID: 551, Title: "Second", Popularity: 5}, {SourceID: 999, Title: "Gone", Popularity: 3},
	}))

	var seen []RowResult
	stats, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Classes: []store.WorkClass{store.WorkUnhydrated}, OnRow: func(r RowResult) { seen = append(seen, r) }})
	require.NoError(t, err)
	assert.Equal(t, HydrateStats{Attempted: 3, OK: 2, Gone: 1, PeopleFetched: 3}, stats)
	assert.Len(t, seen, 3)
	assert.Equal(t, 550, seen[0].SourceID, "most popular first")

	m, err := movies.GetBySourceID(ctx, 550)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d-fight-club", m.ID), m.Slug)
	assert.Equal(t, 139, *m.RuntimeMinutes)
	assert.Equal(t, []string{"Drama"}, m.Genres)
	assert.Equal(t, 1999, m.ReleaseDate.Year())

	var payload tmdb.Movie
	require.NoError(t, json.Unmarshal(m.Payload, &payload))
	assert.Equal(t, m.Title, payload.Title, "typed columns are a projection of the payload")
	assert.Equal(t, m.SourceID, payload.ID, "the provider's id lives in source_id and the payload only")
	assert.Equal(t, []string{"US"}, keys(payload.WatchProviders.Results), "providers pruned to US")
	assert.Len(t, payload.Credits.Cast, 2)

	gone, err := movies.GetBySourceID(ctx, 999)
	require.NoError(t, err)
	assert.Nil(t, gone, "a 404 deletes the row")

	dates, err := people.GetDates(ctx, []int{819, 287, 7467, 7474})
	require.NoError(t, err)
	for _, sourceID := range []int{819, 287, 7467} {
		assert.True(t, dates[sourceID].Fetched, "person %d hydrated", sourceID)
	}
	assert.False(t, dates[7474].Fetched, "crew beyond the director and writers stays at list grade")
	assert.Equal(t, "1962-08-28", dates[7467].DateOfBirth)
	assert.Equal(t, 3, f.count("person"), "each person fetched once")

	var count int
	require.NoError(t, testPool.QueryRow(ctx, `select count(*) from movies where payload is not null and title <> payload->>'title'`).Scan(&count))
	assert.Zero(t, count)
}

func TestHydrate_PeopleFetchHonoursPriorityAndCap(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	f.movies[550] = fightClub()
	for id, name := range map[int]string{819: "Edward Norton", 287: "Brad Pitt", 7467: "David Fincher", 7474: "Art Linson"} {
		f.people[id] = person(id, name, "1970-01-01")
	}
	svc := newTestService(t, f, 1)
	movies := store.NewMovieStore(testPool)
	people := store.NewPersonStore(testPool)
	require.NoError(t, movies.UpsertSkeleton(ctx, testPool, []store.Skeleton{{SourceID: 550, Popularity: 61}}))

	stats, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Classes: []store.WorkClass{store.WorkUnhydrated}})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.PeopleFetched)

	dates, err := people.GetDates(ctx, []int{819, 287, 7467, 7474})
	require.NoError(t, err)
	assert.True(t, dates[7467].Fetched, "the director outranks the cast")
	assert.False(t, dates[819].Fetched)
	assert.Equal(t, "Edward Norton", nameOf(t, 819), "everyone else is seeded at list grade")

	stats, err = svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Classes: []store.WorkClass{store.WorkUnhydrated}})
	require.NoError(t, err)
	assert.Zero(t, stats.Attempted, "hydrated rows are not claimed again")
}

func TestHydrate_StopsAfterConsecutiveFailures(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	svc := newTestService(t, f, 10)
	movies := store.NewMovieStore(testPool)

	rows := make([]store.Skeleton, 0, 30)
	for id := 1; id <= 30; id++ {
		rows = append(rows, store.Skeleton{SourceID: id, Popularity: float64(id)})
		f.failing["movie/"+itoa(id)] = true
	}
	require.NoError(t, movies.UpsertSkeleton(ctx, testPool, rows))

	stats, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Classes: []store.WorkClass{store.WorkUnhydrated}})
	assert.ErrorIs(t, err, ErrTooManyFailures)
	assert.Equal(t, 20, stats.Failed)
	assert.LessOrEqual(t, stats.Attempted, 25, "gives up within the first batch")
}

func TestHydrate_BudgetIsHonoured(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	svc := newTestService(t, f, 10)
	movies := store.NewMovieStore(testPool)
	rows := make([]store.Skeleton, 0, 5)
	for id := 1; id <= 5; id++ {
		f.movies[id] = tmdb.Movie{ID: id, Title: "M" + itoa(id), Popularity: float64(id)}
		rows = append(rows, store.Skeleton{SourceID: id, Popularity: float64(id)})
	}
	require.NoError(t, movies.UpsertSkeleton(ctx, testPool, rows))

	stats, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Budget: 2, Classes: []store.WorkClass{store.WorkUnhydrated}})
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Attempted)
	assert.Equal(t, 2, f.count("movie"))
}

func TestHydrate_SeriesWritesSkeletonSeasonsAndSeasonsWriteSkeletonEpisodes(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	f.series[1396] = breakingBad()
	f.seasons["1396/1"] = breakingBadSeasonOne()
	f.people[17419] = person(17419, "Bryan Cranston", "1956-03-07")
	f.people[66633] = person(66633, "Vince Gilligan", "1967-02-10")
	svc := newTestService(t, f, 0)
	series := store.NewSeriesStore(testPool)
	seasons := store.NewSeasonStore(testPool)
	episodes := store.NewEpisodeStore(testPool)
	require.NoError(t, series.UpsertSkeleton(ctx, testPool, []store.Skeleton{{SourceID: 1396, Title: "Breaking Bad", Popularity: 100}}))

	_, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntitySeries, Classes: []store.WorkClass{store.WorkUnhydrated}})
	require.NoError(t, err)

	row, err := series.GetBySourceID(ctx, 1396)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, fmt.Sprintf("%d-breaking-bad", row.ID), row.Slug)
	assert.Equal(t, 20, *row.TotalEpisodes)
	assert.Equal(t, 47, *row.EpisodeRunTime)
	assert.Equal(t, "Bryan Cranston", nameOf(t, 17419))

	listed, err := seasons.ListBySeries(ctx, row.ID)
	require.NoError(t, err)
	require.Len(t, listed, 2, "the series document writes a skeleton row per season")
	assert.Equal(t, 3572, listed[0].SourceID)
	assert.Nil(t, listed[0].Payload)

	sd, err := svc.Season(ctx, row.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, "Season 1", sd.Name)
	require.NoError(t, svc.bg.Wait(ctx))

	season, err := seasons.Get(ctx, row.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, listed[0].ID, season.ID, "hydration fills the skeleton's row")
	assert.NotNil(t, season.Payload)

	pilot, err := episodes.Get(ctx, row.ID, 1, 1)
	require.NoError(t, err)
	require.NotNil(t, pilot, "the season document writes a skeleton row per episode")
	assert.Equal(t, 62085, pilot.SourceID)
	assert.Nil(t, pilot.Payload)
	assert.Equal(t, "Guest Star", nameOf(t, 1223), "guest stars are seeded at list grade")
}

func TestFetchPerson_SeedsFilmographyAtListGrade(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	p := person(287, "Brad Pitt", "1963-12-18")
	p.CombinedCredits = tmdb.CombinedCredits{
		Cast: []tmdb.CombinedCastCredit{
			{CombinedCreditBase: tmdb.CombinedCreditBase{ID: 550, MediaType: "movie", PosterPath: "/fc.jpg", VoteAverage: 8.4,
				CombinedCreditMovie: tmdb.CombinedCreditMovie{Title: "Fight Club", ReleaseDate: "1999-10-15"}}, Popularity: 61},
			{CombinedCreditBase: tmdb.CombinedCreditBase{ID: 1399, MediaType: "tv",
				CombinedCreditShow: tmdb.CombinedCreditShow{Name: "Friends", FirstAirDate: "1994-09-22"}}, Popularity: 200},
		},
	}
	f.people[287] = p
	svc := newTestService(t, f, 0)
	people := store.NewPersonStore(testPool)
	require.NoError(t, people.UpsertSkeleton(ctx, testPool, []store.PersonSkeleton{{SourceID: 287, Name: "Brad Pitt", Popularity: 30}}))

	_, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityPerson, Classes: []store.WorkClass{store.WorkUnhydrated}})
	require.NoError(t, err)

	got, err := people.GetBySourceID(ctx, 287)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1963, got.DateOfBirth.Year())
	assert.Equal(t, fmt.Sprintf("%d-brad-pitt", got.ID), got.Slug)

	m, err := store.NewMovieStore(testPool).GetBySourceID(ctx, 550)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "/fc.jpg", *m.PosterPath)
	assert.Nil(t, m.Payload)
	s, err := store.NewSeriesStore(testPool).GetBySourceID(ctx, 1399)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "Friends", s.Name)
}

func TestSeedExports_SkipsAndResumes(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	svc := newTestService(t, newFakeTMDB(t), 0)
	people := store.NewPersonStore(testPool)

	lines := []string{
		`{"adult":false,"id":1,"name":"Popular Person","popularity":12.5}`,
		`{"adult":true,"id":2,"name":"Adult Person","popularity":50}`,
		`{"adult":false,"id":3,"name":"Obscure Person","popularity":0.2}`,
		`{"adult":false,"id":4,"name":"Borderline","popularity":0.7}`,
		`{"adult":false,"id":5,"name":"Another","popularity":3}`,
	}
	var batches []BatchResult
	err := svc.SeedExports(ctx, EntityPerson, gzipLines(t, lines), 0, func(b BatchResult) { batches = append(batches, b) })
	require.NoError(t, err)
	require.Len(t, batches, 1)
	assert.Equal(t, BatchResult{LinesRead: 5, Upserted: 4, Skipped: 1}, batches[0])
	assert.Equal(t, "Popular Person", nameOf(t, 1))
	assert.Equal(t, "Obscure Person", nameOf(t, 3), "save has no popularity floor; hydrate does")
	dates, err := people.GetDates(ctx, []int{2})
	require.NoError(t, err)
	assert.Empty(t, dates, "adult rows are never saved")

	batches = nil
	err = svc.SeedExports(ctx, EntityPerson, gzipLines(t, lines), 4, func(b BatchResult) { batches = append(batches, b) })
	require.NoError(t, err)
	require.Len(t, batches, 1)
	assert.Equal(t, BatchResult{LinesRead: 5, Upserted: 1, Skipped: 0}, batches[0], "resume reads past the checkpoint")

	movieLines := []string{`{"adult":false,"id":550,"original_title":"Fight Club","popularity":61.4,"video":false}`}
	require.NoError(t, svc.SeedExports(ctx, EntityMovie, gzipLines(t, movieLines), 0, nil))
	m, err := store.NewMovieStore(testPool).GetBySourceID(ctx, 550)
	require.NoError(t, err)
	assert.Equal(t, "Fight Club", m.Title)
	assert.Equal(t, fmt.Sprintf("%d-fight-club", m.ID), m.Slug, "export rows get their slug at save time")
	assert.InDelta(t, 61.4, m.Popularity, 0.01)
}

func gzipLines(t *testing.T, lines []string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	for _, l := range lines {
		_, err := w.Write([]byte(l + "\n"))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return &buf
}

func nameOf(t *testing.T, personSourceID int) string {
	t.Helper()
	p, err := store.NewPersonStore(testPool).GetBySourceID(context.Background(), personSourceID)
	require.NoError(t, err)
	require.NotNil(t, p, "person %d should exist", personSourceID)
	return p.Name
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestSyncChanges_FlagsHeldRowsForRefresh(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	f.movies[550] = fightClub()
	f.movies[551] = tmdb.Movie{ID: 551, Title: "Second", Popularity: 5}
	f.changes["movie"] = []int{550, 551, 999999}
	svc := newTestService(t, f, 0)
	movies := store.NewMovieStore(testPool)

	_, err := movies.UpsertHydrated(ctx, testPool, store.Movie{SourceID: 550, Title: "Fight Club", Payload: json.RawMessage(`{"id":550}`)})
	require.NoError(t, err)
	require.NoError(t, movies.UpsertSkeleton(ctx, testPool, []store.Skeleton{{SourceID: 551, Title: "Second", Popularity: 5}}))

	changed, marked, err := svc.SyncChanges(ctx, EntityMovie, "2026-09-01", "2026-09-02")
	require.NoError(t, err)
	assert.Equal(t, 3, changed, "everything the feed listed")
	assert.Equal(t, int64(1), marked, "only the hydrated row is flagged; skeletons and unknown ids are not refresh work")

	stats, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Classes: []store.WorkClass{store.WorkStale}})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.OK, "a flagged skeleton is not refreshed; only hydrated rows are")
	row, err := movies.GetBySourceID(ctx, 550)
	require.NoError(t, err)
	assert.False(t, row.Stale)
	assert.Equal(t, 2, f.count("movie"), "one changes page, one refresh")
}

func TestHydrate_FailedRowIsClaimedOncePerRun(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	f.movies[550] = fightClub()
	f.failing["movie/1"] = true
	svc := newTestService(t, f, 10)
	movies := store.NewMovieStore(testPool)
	require.NoError(t, movies.UpsertSkeleton(ctx, testPool, []store.Skeleton{
		{SourceID: 1, Title: "Poison", Popularity: 99},
		{SourceID: 550, Title: "Fight Club", Popularity: 61},
	}))

	stats, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Classes: []store.WorkClass{store.WorkUnhydrated}})
	require.NoError(t, err, "one bad row does not trip the failure streak")
	assert.Equal(t, 2, stats.Attempted)
	assert.Equal(t, 1, stats.Failed)
	assert.Equal(t, 1, stats.OK)
	assert.Equal(t, 2, f.count("movie"), "the failing row is fetched once, not until the streak trips")
}
