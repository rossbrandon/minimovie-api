package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"pgvector/pgvector:0.8.6-pg18",
		postgres.WithInitScripts("../../local-development/init.sql"),
		postgres.WithDatabase("minimovie_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		panic(err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testPool.Close()
	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

func truncateAll(t *testing.T) {
	t.Helper()
	for _, table := range []string{"movies", "series", "seasons", "episodes", "collections", "people"} {
		_, err := testPool.Exec(context.Background(), "delete from "+table)
		require.NoError(t, err)
	}
}

type fakeTMDB struct {
	mu          sync.Mutex
	srv         *httptest.Server
	movies      map[int]tmdb.Movie
	series      map[int]tmdb.Series
	seasons     map[string]tmdb.SeasonDetails
	episodes    map[string]tmdb.EpisodeDetails
	people      map[int]tmdb.Person
	collections map[int]tmdb.Collection
	changes     map[string][]int
	failing     map[string]bool
	calls       map[string]int
	hold        chan struct{} // when set, every response waits until it is closed
}

func newFakeTMDB(t *testing.T) *fakeTMDB {
	t.Helper()
	f := &fakeTMDB{
		movies:      map[int]tmdb.Movie{},
		series:      map[int]tmdb.Series{},
		seasons:     map[string]tmdb.SeasonDetails{},
		episodes:    map[string]tmdb.EpisodeDetails{},
		people:      map[int]tmdb.Person{},
		collections: map[int]tmdb.Collection{},
		changes:     map[string][]int{},
		failing:     map[string]bool{},
		calls:       map[string]int{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTMDB) client() *tmdb.Client {
	return tmdb.NewClient(tmdb.Config{BaseURL: f.srv.URL, Timeout: 5, AccessToken: "test"})
}

func (f *fakeTMDB) handle(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	entity := parts[0]
	id, _ := strconv.Atoi(parts[1])
	key := entity + "/" + parts[1]

	f.mu.Lock()
	f.calls[entity]++
	failing := f.failing[key]
	var body any
	var ok bool
	switch {
	case parts[1] == "changes":
		body, ok = changesBody(f.changes[entity]), true
	case entity == "tv" && len(parts) == 6 && parts[4] == "episode":
		f.calls["episode"]++
		body, ok = f.episodes[parts[1]+"/"+parts[3]+"/"+parts[5]]
	case entity == "tv" && len(parts) == 4 && parts[2] == "season":
		f.calls["season"]++
		body, ok = f.seasons[parts[1]+"/"+parts[3]]
	case entity == "movie":
		body, ok = f.movies[id]
	case entity == "tv":
		body, ok = f.series[id]
	case entity == "person":
		body, ok = f.people[id]
	case entity == "collection":
		body, ok = f.collections[id]
	}
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}

	switch {
	case failing:
		w.WriteHeader(http.StatusTeapot)
	case !ok:
		w.WriteHeader(http.StatusNotFound)
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}

func changesBody(ids []int) map[string]any {
	results := make([]map[string]int, 0, len(ids))
	for _, id := range ids {
		results = append(results, map[string]int{"id": id})
	}
	return map[string]any{"results": results, "page": 1, "total_pages": 1}
}

func (f *fakeTMDB) count(entity string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[entity]
}

// holdResponses blocks every response until the returned release is called.
func (f *fakeTMDB) holdResponses() (release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hold = make(chan struct{})
	return sync.OnceFunc(func() { close(f.hold) })
}

func (f *fakeTMDB) resetCounts() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = map[string]int{}
}

// newTestService caps both the hydrator's and a read's synchronous people fetch at peopleCap.
func newTestService(t *testing.T, f *fakeTMDB, peopleCap int) *Service {
	t.Helper()
	svc := New(Deps{
		Pool:               testPool,
		Movies:             store.NewMovieStore(testPool),
		Series:             store.NewSeriesStore(testPool),
		Seasons:            store.NewSeasonStore(testPool),
		Episodes:           store.NewEpisodeStore(testPool),
		People:             store.NewPersonStore(testPool),
		Collections:        store.NewCollectionStore(testPool),
		TMDB:               f.client(),
		PeoplePerHydration: peopleCap,
		MaxFetchPerRequest: peopleCap,
	})
	t.Cleanup(svc.Stop)
	return svc
}

func fightClub() tmdb.Movie {
	return tmdb.Movie{
		ID: 550, Title: "Fight Club", OriginalTitle: "Fight Club", Overview: "An insomniac office worker...",
		ReleaseDate: "1999-10-15", Runtime: 139, Popularity: 61.4, VoteAverage: 8.4, PosterPath: "/fc.jpg",
		Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}},
		Credits: tmdb.Credits{
			Cast: []tmdb.CastMember{
				{ID: 819, Name: "Edward Norton", Order: 0, Popularity: 20, KnownForDepartment: "Acting"},
				{ID: 287, Name: "Brad Pitt", Order: 1, Popularity: 30, KnownForDepartment: "Acting"},
			},
			Crew: []tmdb.CrewMember{
				{ID: 7467, Name: "David Fincher", Job: tmdb.JobDirector, Popularity: 15},
				{ID: 7474, Name: "Art Linson", Job: tmdb.JobProducer, Popularity: 2},
			},
		},
		WatchProviders: tmdb.WatchProviders{Results: map[string]tmdb.CountryProviders{
			"US": {Link: "us"}, "GB": {Link: "gb"},
		}},
	}
}

func person(id int, name, birthday string) tmdb.Person {
	return tmdb.Person{ID: id, Name: name, Birthday: birthday, Popularity: 10, KnownForDepartment: "Acting"}
}

func breakingBad() tmdb.Series {
	return tmdb.Series{
		ID: 1396, Name: "Breaking Bad", FirstAirDate: "2008-01-20", NumberOfSeasons: 2, NumberOfEpisodes: 20,
		EpisodeRunTime: []int{47}, Popularity: 100, Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}},
		CreatedBy: []tmdb.Creator{{ID: 66633, Name: "Vince Gilligan"}},
		Seasons: []tmdb.Season{
			{ID: 3572, SeasonNumber: 1, Name: "Season 1", EpisodeCount: 7},
			{ID: 3573, SeasonNumber: 2, Name: "Season 2", EpisodeCount: 13},
		},
		AggregateCredits: tmdb.AggregateCredits{
			Cast: []tmdb.AggregateCastMember{{ID: 17419, Name: "Bryan Cranston", TotalEpisodeCount: 20}},
			Crew: []tmdb.AggregateCrewMember{
				{ID: 66633, Name: "Vince Gilligan", Jobs: []tmdb.Job{{Job: tmdb.JobExecutiveProducer}}},
			},
		},
	}
}

func breakingBadSeasonOne() tmdb.SeasonDetails {
	return tmdb.SeasonDetails{
		ID: 3572, SeasonNumber: 1, Name: "Season 1",
		Episodes: []tmdb.Episode{
			{
				ID: 62085, EpisodeNumber: 1, Name: "Pilot", Runtime: 58,
				GuestStars: []tmdb.CastMember{{ID: 1223, Name: "Guest Star"}},
			},
			{ID: 62086, EpisodeNumber: 2, Name: "Cat's in the Bag...", Runtime: 48},
		},
		AggregateCredits: tmdb.AggregateCredits{
			Cast: []tmdb.AggregateCastMember{{ID: 17419, Name: "Bryan Cranston", TotalEpisodeCount: 7}},
		},
	}
}

func breakingBadPilot() tmdb.EpisodeDetails {
	return tmdb.EpisodeDetails{
		ID: 62085, Name: "Pilot", SeasonNumber: 1, EpisodeNumber: 1, Runtime: 58,
		Credits: tmdb.EpisodeCredits{GuestStars: []tmdb.CastMember{{ID: 1223, Name: "Guest Star"}}},
	}
}
