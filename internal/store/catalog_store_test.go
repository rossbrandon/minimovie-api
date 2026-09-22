package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strp(s string) *string { return &s }

func testMovie(sourceID int, title string) Movie {
	payload, _ := json.Marshal(map[string]any{"id": sourceID, "title": title})
	release := time.Date(1999, 10, 15, 0, 0, 0, 0, time.UTC)
	return Movie{
		SourceID: sourceID, Title: title, OriginalTitle: strp(title), ReleaseDate: &release,
		Popularity: 10, Genres: []string{"Drama"}, Payload: payload,
	}
}

func TestMovieStore_UpsertHydratedAndGet(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewMovieStore(testPool)

	id, err := s.UpsertHydrated(ctx, testPool, testMovie(550, "Fight Club"))
	require.NoError(t, err)
	require.Positive(t, id)

	got, err := s.GetByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 550, got.SourceID)
	assert.Equal(t, "Fight Club", got.Title)
	assert.Equal(t, fmt.Sprintf("%d-fight-club", id), got.Slug, "slug is generated from our id and the title")
	assert.Equal(t, []string{"Drama"}, got.Genres)
	assert.JSONEq(t, `{"id":550,"title":"Fight Club"}`, string(got.Payload))
	assert.False(t, got.Stale)
	require.NotNil(t, got.FetchedAt)
	assert.WithinDuration(t, time.Now(), *got.FetchedAt, time.Minute)

	bySource, err := s.GetBySourceID(ctx, 550)
	require.NoError(t, err)
	require.NotNil(t, bySource)
	assert.Equal(t, id, bySource.ID)

	ids, err := s.IDsBySource(ctx, []int{550, 999})
	require.NoError(t, err)
	assert.Equal(t, map[int]int{550: id}, ids, "an id without a row is absent")
	ids, err = s.IDsBySource(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, ids)

	again, err := s.UpsertHydrated(ctx, testPool, testMovie(550, "Fight Club"))
	require.NoError(t, err)
	assert.Equal(t, id, again, "upserting the same source keeps the same row")

	missing, err := s.GetByID(ctx, 999999)
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestSlug_FollowsIdAndTitle(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewMovieStore(testPool)

	cases := map[string]string{
		"Amélie":       "amelie",
		"Se7en":        "se7en",
		"The Thing":    "the-thing",
		"日本語":          "",
		"  Spaced  --": "spaced",
	}
	for title, want := range cases {
		id, err := s.UpsertHydrated(ctx, testPool, testMovie(len(title)*7+1, title))
		require.NoError(t, err)
		row, _ := s.GetByID(ctx, id)
		expected := fmt.Sprintf("%d-%s", id, want)
		if want == "" {
			expected = fmt.Sprint(id)
		}
		assert.Equal(t, expected, row.Slug, title)
	}

	id, err := s.UpsertHydrated(ctx, testPool, testMovie(194, "Le Fabuleux Destin d'Amélie Poulain"))
	require.NoError(t, err)
	_, err = s.UpsertHydrated(ctx, testPool, testMovie(194, "Amélie"))
	require.NoError(t, err)
	row, _ := s.GetByID(ctx, id)
	assert.Equal(t, fmt.Sprintf("%d-amelie", id), row.Slug, "a renamed title moves the slug; the leading id still resolves")
}

func TestMovieStore_UpsertSkeletonNeverClobbersHydratedRows(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewMovieStore(testPool)

	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 10, Title: "Export Title", Popularity: 1}}))
	row, _ := s.GetBySourceID(ctx, 10)
	assert.Equal(t, "Export Title", row.Title)
	assert.Equal(t, fmt.Sprintf("%d-export-title", row.ID), row.Slug, "skeleton rows get a slug too")
	assert.Nil(t, row.Payload)
	assert.Nil(t, row.PosterPath)

	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 10, Title: "List Title", Popularity: 2, PosterPath: strp("/p.jpg"), ReleaseDate: strp("2001-02-03")}}))
	row, _ = s.GetBySourceID(ctx, 10)
	assert.Equal(t, "List Title", row.Title)
	assert.Equal(t, "/p.jpg", *row.PosterPath)
	assert.Equal(t, 2001, row.ReleaseDate.Year())
	assert.Nil(t, row.FetchedAt, "only a hydration stamps fetched_at")

	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 10, Title: "Again", Popularity: 3}}))
	row, _ = s.GetBySourceID(ctx, 10)
	assert.Equal(t, "/p.jpg", *row.PosterPath, "a nil field never overwrites a stored value")

	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 10, Title: "Part"}}))
	row, _ = s.GetBySourceID(ctx, 10)
	assert.Equal(t, 3.0, row.Popularity, "a list without popularity (collection parts) never zeroes an export value")

	_, err := s.UpsertHydrated(ctx, testPool, testMovie(10, "Hydrated Title"))
	require.NoError(t, err)
	hydrated, _ := s.GetBySourceID(ctx, 10)

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 10, Title: "Export Again", Popularity: 4, Overview: strp("x")}}))
	after, _ := s.GetBySourceID(ctx, 10)
	assert.Equal(t, "Hydrated Title", after.Title, "hydrated fields are kept")
	assert.Nil(t, after.Overview)
	assert.Equal(t, hydrated.Popularity, after.Popularity, "a hydrated row keeps the API's popularity; exports and credit lists no longer move it")
	assert.Equal(t, hydrated.FetchedAt, after.FetchedAt, "a skeleton refresh never extends a payload's life")
	assert.True(t, after.UpdatedAt.After(hydrated.UpdatedAt), "every write bumps updated_at")
	assert.Equal(t, hydrated.ID, after.ID)
}

func TestPersonStore_GetDatesReportsHydration(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewPersonStore(testPool)

	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []PersonSkeleton{{SourceID: 1, Name: "List Grade", Popularity: 3}}))
	dob := time.Date(1963, 12, 18, 0, 0, 0, 0, time.UTC)
	id, err := s.UpsertHydrated(ctx, testPool, Person{SourceID: 2, Name: "Brad Pitt", DateOfBirth: &dob, Popularity: 50, Payload: json.RawMessage(`{"id":2}`)})
	require.NoError(t, err)
	_, err = s.UpsertHydrated(ctx, testPool, Person{SourceID: 3, Name: "Unknown Birthday", Payload: json.RawMessage(`{"id":3}`)})
	require.NoError(t, err)

	dates, err := s.GetDates(ctx, []int{1, 2, 3, 4})
	require.NoError(t, err)
	assert.False(t, dates[1].Fetched)
	assert.True(t, dates[2].Fetched)
	assert.Equal(t, id, dates[2].ID)
	assert.Equal(t, "1963-12-18", dates[2].DateOfBirth)
	assert.True(t, dates[3].Fetched, "a hydrated person with no birthday is not a gap")
	assert.Empty(t, dates[3].DateOfBirth)
	_, ok := dates[4]
	assert.False(t, ok)

	p, err := s.GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d-brad-pitt", id), p.Slug)
}

func TestPersonStore_InsightsRoundTrip(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewPersonStore(testPool)
	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []PersonSkeleton{{SourceID: 1, Name: "Someone"}}))
	p, err := s.GetBySourceID(ctx, 1)
	require.NoError(t, err)

	data, at, err := s.GetInsights(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Nil(t, at)

	require.NoError(t, s.SetInsights(ctx, p.ID, json.RawMessage(`{"netWorth":"lots"}`)))
	data, at, err = s.GetInsights(ctx, p.ID)
	require.NoError(t, err)
	assert.JSONEq(t, `{"netWorth":"lots"}`, string(data))
	require.NotNil(t, at)
}

func TestCatalogTable_DeleteExpiredBoundary(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewMovieStore(testPool)
	_, err := s.UpsertHydrated(ctx, testPool, testMovie(1, "Old"))
	require.NoError(t, err)
	_, err = s.UpsertHydrated(ctx, testPool, testMovie(2, "Fresh"))
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `update movies set fetched_at = now() - interval '181 days' where source_id = 1`)
	require.NoError(t, err)

	deleted, err := s.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	fresh, _ := s.GetBySourceID(ctx, 2)
	assert.NotNil(t, fresh)
}

func TestCatalogTable_MarkStaleAndClaimByClass(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewMovieStore(testPool)
	_, err := s.UpsertHydrated(ctx, testPool, testMovie(1, "Hydrated Stale"))
	require.NoError(t, err)
	_, err = s.UpsertHydrated(ctx, testPool, testMovie(2, "Hydrated Old"))
	require.NoError(t, err)
	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 3, Title: "Popular", Popularity: 9}, {SourceID: 4, Title: "Obscure", Popularity: 1}}))
	_, err = testPool.Exec(ctx, `update movies set fetched_at = now() - interval '151 days' where source_id = 2`)
	require.NoError(t, err)

	n, err := s.MarkStale(ctx, []int{1, 3, 99})
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "only the hydrated row is flagged; the skeleton and the unknown id are not refresh work")

	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck

	params := ClaimParams{Limit: 10, RefreshAfter: RefreshAfter, MinPopularity: 5}
	stale, err := s.Claim(ctx, tx, WorkStale, params)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, stale, "claims return source ids; stale skeletons are not refresh work")

	expiring, err := s.Claim(ctx, tx, WorkExpiring, params)
	require.NoError(t, err)
	assert.Equal(t, []int{2}, expiring)

	unhydrated, err := s.Claim(ctx, tx, WorkUnhydrated, params)
	require.NoError(t, err)
	assert.Equal(t, []int{3}, unhydrated, "below the popularity floor is left alone")
}

func TestCatalogTable_ClaimSkipsRowsLockedByAnotherRun(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewMovieStore(testPool)
	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 1, Popularity: 30}, {SourceID: 2, Popularity: 20}, {SourceID: 3, Popularity: 10}}))

	tx1, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx1.Rollback(ctx) //nolint:errcheck
	tx2, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx) //nolint:errcheck

	params := ClaimParams{Limit: 2}
	first, err := s.Claim(ctx, tx1, WorkUnhydrated, params)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, first, "most popular first")

	second, err := s.Claim(ctx, tx2, WorkUnhydrated, params)
	require.NoError(t, err)
	assert.Equal(t, []int{3}, second, "a concurrent run sees only what is unlocked")
}

func TestCatalogTable_Stats(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewMovieStore(testPool)
	_, err := s.UpsertHydrated(ctx, testPool, testMovie(1, "A"))
	require.NoError(t, err)
	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 2, Popularity: 9}, {SourceID: 3, Popularity: 1}}))
	_, err = s.MarkStale(ctx, []int{1})
	require.NoError(t, err)

	stats, err := s.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, CatalogStats{Table: "movies", Rows: 3, Hydrated: 1, Stale: 1, Expiring: 0, UnhydratedPopular: 1, OldestDays: 0}, stats)
}

func TestSeasonStore_SkeletonsThenPayload(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	series := NewSeriesStore(testPool)
	seasons := NewSeasonStore(testPool)
	seriesID, err := series.UpsertHydrated(ctx, testPool, Series{SourceID: 1396, Name: "Breaking Bad", Payload: json.RawMessage(`{"id":1396}`)})
	require.NoError(t, err)

	require.NoError(t, seasons.UpsertSkeletons(ctx, testPool, seriesID, []SeasonSkeleton{
		{SeasonNumber: 1, SourceID: 3572, Name: "Season 1"}, {SeasonNumber: 2, SourceID: 3573, Name: "Season 2"},
	}))
	listed, err := seasons.ListBySeries(ctx, seriesID)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Nil(t, listed[0].Payload, "skeleton seasons have no payload yet")
	assert.Equal(t, "Season 1", listed[0].Name)
	ids, err := seasons.IDsBySeries(ctx, seriesID)
	require.NoError(t, err)
	assert.Equal(t, map[int]int{1: listed[0].ID, 2: listed[1].ID}, ids)

	payload := json.RawMessage(`{
		"id": 3572, "season_number": 1, "name": "Season One",
		"episodes": [{"episode_number": 1, "runtime": 58}, {"episode_number": 2, "runtime": 48}, {"episode_number": 3}],
		"aggregate_credits": {"cast": [{"id": 17419, "total_episode_count": 7}, {"id": 84497, "total_episode_count": 6}]}
	}`)
	id, err := seasons.Upsert(ctx, testPool, Season{SeriesID: seriesID, SeasonNumber: 1, SourceID: 3572, Name: "Season One", Payload: payload})
	require.NoError(t, err)
	assert.Equal(t, listed[0].ID, id, "hydration keeps the skeleton's row")

	got, err := seasons.Get(ctx, seriesID, 1)
	require.NoError(t, err)
	assert.Equal(t, "Season One", got.Name)
	require.NoError(t, seasons.Delete(ctx, testPool, seriesID, 2))
	listed, _ = seasons.ListBySeries(ctx, seriesID)
	assert.Len(t, listed, 1)
}

func TestSeriesStore_UpsertHydratedAndSkeleton(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewSeriesStore(testPool)

	require.NoError(t, s.UpsertSkeleton(ctx, testPool, []Skeleton{{SourceID: 1396, Title: "Breaking Bad", Popularity: 90}}))
	row, err := s.GetBySourceID(ctx, 1396)
	require.NoError(t, err)
	assert.Nil(t, row.TotalEpisodes, "totals stay null until hydrated")
	assert.Equal(t, fmt.Sprintf("%d-breaking-bad", row.ID), row.Slug)

	inProduction, total := false, 62
	id, err := s.UpsertHydrated(ctx, testPool, Series{SourceID: 1396, Name: "Breaking Bad",
		InProduction: &inProduction, TotalEpisodes: &total, Popularity: 95, Payload: json.RawMessage(`{"id":1396}`)})
	require.NoError(t, err)
	assert.Equal(t, row.ID, id)
	row, err = s.GetByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 62, *row.TotalEpisodes)
	assert.False(t, *row.InProduction)
}

func TestEpisodeAndCollectionStores_RoundTrip(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	es := NewEpisodeStore(testPool)
	cs := NewCollectionStore(testPool)

	require.NoError(t, es.UpsertSkeletons(ctx, testPool, 7, 1, []EpisodeSkeleton{{EpisodeNumber: 1, SourceID: 62085, Name: "Pilot"}, {EpisodeNumber: 2, SourceID: 62086, Name: "Cat's in the Bag..."}}))
	ep, err := es.Get(ctx, 7, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 62085, ep.SourceID)
	assert.Nil(t, ep.Payload)
	ep2, err := es.Get(ctx, 7, 1, 2)
	require.NoError(t, err)
	ids, err := es.IDsBySeason(ctx, 7, 1)
	require.NoError(t, err)
	assert.Equal(t, map[int]int{1: ep.ID, 2: ep2.ID}, ids)

	id, err := es.Upsert(ctx, testPool, Episode{SeriesID: 7, SeasonNumber: 1, EpisodeNumber: 1, SourceID: 62085, Name: "Pilot", Payload: json.RawMessage(`{"name":"Pilot"}`)})
	require.NoError(t, err)
	assert.Equal(t, ep.ID, id)
	require.NoError(t, es.Delete(ctx, testPool, 7, 1, 2))

	cid, err := cs.Upsert(ctx, testPool, Collection{SourceID: 10, Name: "Star Wars Collection", Payload: json.RawMessage(`{"id":10}`)})
	require.NoError(t, err)
	c, err := cs.Get(ctx, cid)
	require.NoError(t, err)
	assert.Equal(t, "Star Wars Collection", c.Name)
	assert.Equal(t, fmt.Sprintf("%d-star-wars-collection", cid), c.Slug)
}

func TestSeasonStore_UpsertSkeletonsSurvivesRenumbering(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewSeasonStore(testPool)

	require.NoError(t, s.UpsertSkeletons(ctx, testPool, 7, []SeasonSkeleton{
		{SeasonNumber: 1, SourceID: 100, Name: "2020"},
		{SeasonNumber: 2, SourceID: 200, Name: "2021"},
	}))
	err := s.UpsertSkeletons(ctx, testPool, 7, []SeasonSkeleton{
		{SeasonNumber: 2, SourceID: 100, Name: "2020"},
		{SeasonNumber: 1, SourceID: 200, Name: "2021"},
	})
	require.NoError(t, err, "TMDB renumbers seasons; uniqueness is checked once the whole statement has run")

	seasons, err := s.ListBySeries(ctx, 7)
	require.NoError(t, err)
	numbers := map[int]int{}
	for _, se := range seasons {
		numbers[se.SourceID] = se.SeasonNumber
	}
	assert.Equal(t, map[int]int{100: 2, 200: 1}, numbers)
}
