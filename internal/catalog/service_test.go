package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fightClubFake serves Fight Club and its four credited people.
func fightClubFake(t *testing.T) *fakeTMDB {
	t.Helper()
	f := newFakeTMDB(t)
	f.movies[550] = fightClub()
	f.people[819] = person(819, "Edward Norton", "1969-08-18")
	f.people[287] = person(287, "Brad Pitt", "1963-12-18")
	f.people[7467] = person(7467, "David Fincher", "1962-08-28")
	f.people[7474] = person(7474, "Art Linson", "")
	return f
}

// hydrateFightClub stores Fight Club and its people through the hydrator and returns the movie's id.
func hydrateFightClub(t *testing.T, ctx context.Context, svc *Service, f *fakeTMDB) int {
	t.Helper()
	id := skeletonMovie(t, ctx, 550, "Fight Club")
	_, err := svc.Hydrate(ctx, HydrateOptions{Entity: EntityMovie, Classes: []store.WorkClass{store.WorkUnhydrated}})
	require.NoError(t, err)
	f.resetCounts()
	return id
}

// skeletonMovie writes one list-grade movie row and returns its id, which a hydration keeps.
func skeletonMovie(t *testing.T, ctx context.Context, sourceID int, title string) int {
	t.Helper()
	movies := store.NewMovieStore(testPool)
	require.NoError(t, movies.UpsertSkeleton(ctx, testPool, []store.Skeleton{{SourceID: sourceID, Title: title}}))
	row, err := movies.GetBySourceID(ctx, sourceID)
	require.NoError(t, err)
	return row.ID
}

func skeletonSeries(t *testing.T, ctx context.Context, sourceID int, title string) int {
	t.Helper()
	series := store.NewSeriesStore(testPool)
	require.NoError(t, series.UpsertSkeleton(ctx, testPool, []store.Skeleton{{SourceID: sourceID, Title: title}}))
	row, err := series.GetBySourceID(ctx, sourceID)
	require.NoError(t, err)
	return row.ID
}

func TestMovie_FreshRowMakesNoProviderCalls(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := fightClubFake(t)
	svc := newTestService(t, f, 10)
	id := hydrateFightClub(t, ctx, svc, f)

	m, err := svc.Movie(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, m.ID)
	assert.Equal(t, fmt.Sprintf("%d-fight-club", id), m.Slug)
	assert.Equal(t, "Fight Club", m.Title)
	assert.Equal(t, "1962-08-28", m.People[7467].DateOfBirth)
	assert.Positive(t, m.People[7474].ID, "a list-grade person still maps to a row")
	assert.False(t, m.People[7474].Fetched)
	assert.Zero(t, f.count("movie"))
	assert.Zero(t, f.count("person"), "the crew beyond the director is left for the fetcher, which is not running")

	_, err = svc.Movie(ctx, id+1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMovie_StaleRowIsServedAndRefreshedOnce(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := fightClubFake(t)
	svc := newTestService(t, f, 10)
	id := hydrateFightClub(t, ctx, svc, f)
	_, err := testPool.Exec(ctx, `update movies set stale = true where id = $1`, id)
	require.NoError(t, err)
	renamed := fightClub()
	renamed.Title = "Fight Club (Remastered)"
	f.movies[550] = renamed
	release := f.holdResponses()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			m, err := svc.Movie(ctx, id)
			if assert.NoError(t, err) {
				assert.Equal(t, "Fight Club", m.Title, "served as stored while the refresh is in flight")
			}
		})
	}
	wg.Wait()
	release()
	require.NoError(t, svc.bg.Wait(ctx))

	assert.Equal(t, 1, f.count("movie"), "ten concurrent stale reads trigger one refresh")
	row, err := store.NewMovieStore(testPool).GetByID(ctx, id)
	require.NoError(t, err)
	assert.False(t, row.Stale)
	assert.Equal(t, "Fight Club (Remastered)", row.Title)
}

func TestMovie_ExpiredRowIsFetchedBeforeServing(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := fightClubFake(t)
	svc := newTestService(t, f, 10)
	id := hydrateFightClub(t, ctx, svc, f)
	_, err := testPool.Exec(ctx, `update movies set fetched_at = now() - interval '181 days' where id = $1`, id)
	require.NoError(t, err)

	m, err := svc.Movie(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Fight Club", m.Title)
	assert.Equal(t, 1, f.count("movie"))
	row, err := store.NewMovieStore(testPool).GetByID(ctx, id)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), *row.FetchedAt, time.Minute)
}

func TestMovie_ConcurrentMissesShareOneFetchAndSeedInBackground(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := fightClubFake(t)
	f.collections[10] = tmdb.Collection{
		ID:    10,
		Name:  "Fight Club Collection",
		Parts: []tmdb.CollectionPart{{ID: 550, Title: "Fight Club"}, {ID: 551, Title: "Fight Club 2"}},
	}
	withCollection := fightClub()
	withCollection.BelongsToCollection = &tmdb.BelongsToCollection{ID: 10, Name: "Fight Club Collection"}
	f.movies[550] = withCollection
	svc := newTestService(t, f, 10)
	id := skeletonMovie(t, ctx, 550, "Fight Club")

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			m, err := svc.Movie(ctx, id)
			if assert.NoError(t, err) {
				assert.Equal(t, 139, m.Runtime, "the document is served on the first read")
				assert.NotNil(t, m.CollectionID, "the collection is written with the movie")
				assert.Equal(t, "1962-08-28", m.People[7467].DateOfBirth, "priority people are fetched before serving")
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 1, f.count("movie"), "concurrent misses coalesce")
	assert.Equal(t, 3, f.count("person"), "director, writers, and top cast once each; the producer waits for the fetcher")

	require.NoError(t, svc.bg.Wait(ctx))
	assert.Equal(t, "Art Linson", nameOf(t, 7474), "the credited people are seeded after the response")
	m, err := svc.Movie(ctx, id)
	require.NoError(t, err)
	c, err := svc.Collection(ctx, *m.CollectionID)
	require.NoError(t, err)
	assert.Equal(t, "Fight Club Collection", c.Name)
	part2, err := store.NewMovieStore(testPool).GetBySourceID(ctx, 551)
	require.NoError(t, err)
	assert.Equal(t, IDs{550: id, 551: part2.ID}, c.Parts, "parts map from the second load on")
}

func TestMovie_GoneAtProviderDeletesTheRow(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	svc := newTestService(t, f, 10)
	id := skeletonMovie(t, ctx, 999, "Gone")

	_, err := svc.Movie(ctx, id)
	assert.ErrorIs(t, err, ErrNotFound)
	row, err := store.NewMovieStore(testPool).GetByID(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestPeople_CapThenFetcherAndNoRefetchOfKnownBlanks(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := fightClubFake(t)
	svc := newTestService(t, f, 1)
	svc.Start()
	people := store.NewPersonStore(testPool)
	id := skeletonMovie(t, ctx, 550, "Fight Club")
	_, err := people.UpsertHydrated(ctx, testPool, store.Person{
		SourceID: 7474,
		Name:     "Art Linson",
		Payload:  json.RawMessage(`{"id":7474}`),
	})
	require.NoError(t, err)

	m, err := svc.Movie(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 1, f.count("person"), "one synchronous fetch, the cap")
	assert.Equal(t, "1962-08-28", m.People[7467].DateOfBirth, "the director outranks the cast")
	assert.False(t, m.People[819].Fetched)

	require.Eventually(t, func() bool {
		dates, err := people.GetDates(ctx, []int{819, 287})
		return err == nil && dates[819].Fetched && dates[287].Fetched
	}, 5*time.Second, 20*time.Millisecond, "the fetcher hydrates the rest")
	assert.Equal(t, 3, f.count("person"), "a hydrated person with no birthday is fetched by neither path")
}

func TestFetcher_StopAndLateEnqueueAreSafe(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := fightClubFake(t)
	svc := newTestService(t, f, 10)
	svc.enqueuePeople([]int{287})
	assert.Zero(t, f.count("person"), "before Start an enqueue is dropped")

	svc.Start()
	svc.enqueuePeople([]int{287})
	require.Eventually(t, func() bool { return f.count("person") == 1 }, 5*time.Second, 20*time.Millisecond)
	svc.Stop()

	ids := make([]int, 2*peopleFetcherQueue)
	svc.enqueuePeople(ids)
	svc.Stop()
	assert.Equal(t, 1, f.count("person"), "after Stop an enqueue fills the buffer, drops the rest, and fetches nothing")
	_, err := svc.Person(ctx, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSeries_MissMapsSeasonsEpisodesAndPeople(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	f.series[1396] = breakingBad()
	f.seasons["1396/1"] = breakingBadSeasonOne()
	f.episodes["1396/1/1"] = breakingBadPilot()
	f.people[17419] = person(17419, "Bryan Cranston", "1956-03-07")
	f.people[66633] = person(66633, "Vince Gilligan", "1967-02-10")
	f.people[1223] = person(1223, "Guest Star", "1980-01-01")
	svc := newTestService(t, f, 10)
	id := skeletonSeries(t, ctx, 1396, "Breaking Bad")

	sr, err := svc.Series(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d-breaking-bad", id), sr.Slug)
	assert.Len(t, sr.Seasons, 2, "season ids are on the first response")
	assert.Equal(t, "1956-03-07", sr.People[17419].DateOfBirth)
	assert.Positive(t, sr.People[66633].ID, "creators are credited too")

	season, err := svc.Season(ctx, id, 1)
	require.NoError(t, err)
	assert.Equal(t, sr.Seasons[1], season.ID)
	assert.Len(t, season.Episodes, 2)
	assert.Equal(t, "1956-03-07", season.People[17419].DateOfBirth)

	ep, err := svc.Episode(ctx, id, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, season.Episodes[1], ep.ID)
	assert.Equal(t, "Pilot", ep.Name)

	docs, err := svc.Seasons(ctx, id, []int{1, 2})
	require.NoError(t, err)
	assert.Len(t, docs, 1, "a season the provider does not have is left out")
	assert.Equal(t, "Season 1", docs[1].Name)
	assert.Equal(t, 2, f.count("season"), "season 1 once, by the first read; season 2 once, found gone")

	require.NoError(t, svc.bg.Wait(ctx))
	season, err = svc.Season(ctx, id, 1)
	require.NoError(t, err)
	guest := season.People[1223]
	assert.Positive(t, guest.ID, "guest stars are seeded at list grade after the response")
	assert.False(t, guest.Fetched, "and left to the fetcher, being cast priority")
}

func TestSeason_StaleRowIsServedAndRefreshed(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	f.series[1396] = breakingBad()
	f.seasons["1396/1"] = breakingBadSeasonOne()
	svc := newTestService(t, f, 10)
	id := skeletonSeries(t, ctx, 1396, "Breaking Bad")
	_, err := svc.Season(ctx, id, 1)
	require.NoError(t, err)
	require.NoError(t, svc.bg.Wait(ctx))
	f.changes["tv"] = []int{1396}

	_, marked, err := svc.SyncChanges(ctx, EntitySeries, "2026-09-01", "2026-09-01")
	require.NoError(t, err)
	assert.Equal(t, int64(1), marked)
	seasons := store.NewSeasonStore(testPool)
	listed, err := seasons.ListBySeries(ctx, id)
	require.NoError(t, err)
	assert.True(t, listed[0].Stale, "the hydrated season follows its series")
	assert.False(t, listed[1].Stale, "a skeleton season has nothing to refresh")
	f.resetCounts()

	season, err := svc.Season(ctx, id, 1)
	require.NoError(t, err)
	assert.Equal(t, "Season 1", season.Name)
	require.NoError(t, svc.bg.Wait(ctx))
	assert.Equal(t, 1, f.count("season"))
	refreshed, err := seasons.Get(ctx, id, 1)
	require.NoError(t, err)
	assert.False(t, refreshed.Stale)
}

func TestPerson_MapsFilmographyOnceSeeded(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	p := person(287, "Brad Pitt", "1963-12-18")
	p.CombinedCredits = tmdb.CombinedCredits{
		Cast: []tmdb.CombinedCastCredit{
			{CombinedCreditBase: tmdb.CombinedCreditBase{ID: 550, MediaType: "movie"}},
			{CombinedCreditBase: tmdb.CombinedCreditBase{ID: 1399, MediaType: "tv"}},
		},
	}
	f.people[287] = p
	svc := newTestService(t, f, 10)
	people := store.NewPersonStore(testPool)
	require.NoError(t, people.UpsertSkeleton(ctx, testPool, []store.PersonSkeleton{{SourceID: 287, Name: "Brad Pitt"}}))
	skeleton, err := people.GetBySourceID(ctx, 287)
	require.NoError(t, err)

	got, err := svc.Person(ctx, skeleton.ID)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%d-brad-pitt", skeleton.ID), got.Slug)
	assert.Equal(t, "1963-12-18", got.Birthday)
	require.NoError(t, svc.bg.Wait(ctx))

	got, err = svc.Person(ctx, skeleton.ID)
	require.NoError(t, err)
	assert.Len(t, got.Movies, 1, "filmography maps once the seed has run")
	assert.Len(t, got.Series, 1)
}

func TestSeedSearch_MapsHeldRowsAndSeedsTheRest(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := newFakeTMDB(t)
	svc := newTestService(t, f, 10)
	held := skeletonMovie(t, ctx, 550, "Fight Club")
	results := &tmdb.SearchResults{Results: []tmdb.SearchResult{
		{ID: 550, MediaType: tmdb.MediaTypeMovie, SearchResultMovie: tmdb.SearchResultMovie{Title: "Fight Club"}},
		{ID: 777, MediaType: tmdb.MediaTypeMovie, SearchResultMovie: tmdb.SearchResultMovie{Title: "New"}, Popularity: 9},
		{ID: 287, MediaType: tmdb.MediaTypePerson, SearchResultShow: tmdb.SearchResultShow{Name: "Brad Pitt"}},
	}}

	refs, err := svc.SeedSearch(ctx, results)
	require.NoError(t, err)
	assert.Equal(t, IDs{550: held}, refs.Movies, "only rows we hold map on this page")
	assert.Empty(t, refs.People)

	require.NoError(t, svc.bg.Wait(ctx))
	refs, err = svc.SeedSearch(ctx, results)
	require.NoError(t, err)
	assert.Len(t, refs.Movies, 2)
	assert.Len(t, refs.People, 1)
	seeded, err := store.NewMovieStore(testPool).GetBySourceID(ctx, 777)
	require.NoError(t, err)
	assert.InDelta(t, 9, seeded.Popularity, 0.01)
	assert.Equal(t, "Brad Pitt", nameOf(t, 287))
}

func TestResolve_SnapshotsForWatchEvents(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	f := fightClubFake(t)
	f.series[1396] = breakingBad()
	f.seasons["1396/1"] = breakingBadSeasonOne()
	f.episodes["1396/1/1"] = breakingBadPilot()
	svc := newTestService(t, f, 10)
	movieID := hydrateFightClub(t, ctx, svc, f)
	seriesID := skeletonSeries(t, ctx, 1396, "Breaking Bad")

	movie, err := svc.ResolveMovie(ctx, movieID)
	require.NoError(t, err)
	assert.Equal(t, store.ResolvedMedia{Title: "Fight Club", Genres: []string{"Drama"}, RuntimeMinutes: ptr(139)}, movie)

	sr, err := svc.ResolveSeries(ctx, seriesID)
	require.NoError(t, err)
	assert.Equal(t, "Breaking Bad", sr.Title)
	assert.Equal(t, 20*47, *sr.RuntimeMinutes)
	assert.Equal(t, 20, *sr.EpisodeCount)
	assert.Equal(t, 2, *sr.SeasonCount)

	season, err := svc.ResolveSeason(ctx, seriesID, 1)
	require.NoError(t, err)
	assert.Equal(t, "Season 1", season.Title)
	assert.Equal(t, "Breaking Bad", *season.SeriesTitle)
	assert.Equal(t, 106, *season.RuntimeMinutes)
	assert.Equal(t, 2, *season.EpisodeCount)

	ep, err := svc.ResolveEpisode(ctx, seriesID, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, "Pilot", ep.Title)
	assert.Equal(t, 58, *ep.RuntimeMinutes)
	assert.Equal(t, []string{"Drama"}, ep.Genres)
	require.NoError(t, svc.bg.Wait(ctx))
}
