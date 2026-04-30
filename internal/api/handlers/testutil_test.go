package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/config"
	mw "github.com/rossbrandon/minimovie-api/internal/api/middleware"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
)

// --- fakeTmdbClient implements tmdb.MediaClient ---

type fakeTmdbClient struct {
	movie         *tmdb.Movie
	movieErr      error
	series        *tmdb.Series
	seriesErr     error
	seriesSeasons *tmdb.SeriesWithSeasons
	season        *tmdb.SeasonDetails
	seasonErr     error
	aggCredits    *tmdb.AggregateCredits
	episode       *tmdb.EpisodeDetails
	episodeErr    error
	person        *tmdb.Person
	personErr     error
	collection    *tmdb.Collection
	searchResults *tmdb.SearchResults
	searchErr     error
}

func (f *fakeTmdbClient) GetMovie(_ context.Context, _ int) (*tmdb.Movie, error) {
	return f.movie, f.movieErr
}
func (f *fakeTmdbClient) GetSeries(_ context.Context, _ int) (*tmdb.Series, error) {
	return f.series, f.seriesErr
}
func (f *fakeTmdbClient) GetSeriesWithSeasons(_ context.Context, _ int) (*tmdb.SeriesWithSeasons, error) {
	return f.seriesSeasons, f.seriesErr
}
func (f *fakeTmdbClient) GetSeason(_ context.Context, _, _ int) (*tmdb.SeasonDetails, error) {
	return f.season, f.seasonErr
}
func (f *fakeTmdbClient) GetSeasonAggregateCredits(_ context.Context, _, _ int) (*tmdb.AggregateCredits, error) {
	return f.aggCredits, nil
}
func (f *fakeTmdbClient) GetEpisode(_ context.Context, _, _, _ int) (*tmdb.EpisodeDetails, error) {
	return f.episode, f.episodeErr
}
func (f *fakeTmdbClient) GetPerson(_ context.Context, _ int) (*tmdb.Person, error) {
	return f.person, f.personErr
}
func (f *fakeTmdbClient) GetCollection(_ context.Context, _ int) (*tmdb.Collection, error) {
	return f.collection, nil
}
func (f *fakeTmdbClient) SearchMulti(_ context.Context, _ string, _ int) (*tmdb.SearchResults, error) {
	return f.searchResults, f.searchErr
}
func (f *fakeTmdbClient) SearchMovies(_ context.Context, _ string, _ int) (*tmdb.SearchResults, error) {
	return f.searchResults, f.searchErr
}
func (f *fakeTmdbClient) SearchSeries(_ context.Context, _ string, _ int) (*tmdb.SearchResults, error) {
	return f.searchResults, f.searchErr
}
func (f *fakeTmdbClient) SearchPerson(_ context.Context, _ string, _ int) (*tmdb.SearchResults, error) {
	return f.searchResults, f.searchErr
}

// --- fake store implementations ---

type fakeUserStore struct {
	user         *store.User
	getUserErr   error
	upsertResult *store.UpsertResult
	upsertErr    error
	accounts     []store.OAuthAccount
	listErr      error
	canExport    bool
	canExportErr error
	deleteErr    error
	markExpErr   error
}

func (f *fakeUserStore) GetByID(_ context.Context, _ string) (*store.User, error) {
	return f.user, f.getUserErr
}
func (f *fakeUserStore) UpsertFromOAuth(_ context.Context, _, _, _ string, _, _ *string) (*store.UpsertResult, error) {
	return f.upsertResult, f.upsertErr
}
func (f *fakeUserStore) ListOAuthAccounts(_ context.Context, _ string) ([]store.OAuthAccount, error) {
	return f.accounts, f.listErr
}
func (f *fakeUserStore) CanExportToday(_ context.Context, _ string) (bool, error) {
	return f.canExport, f.canExportErr
}
func (f *fakeUserStore) MarkExported(_ context.Context, _ string) error { return f.markExpErr }
func (f *fakeUserStore) Delete(_ context.Context, _ string) error       { return f.deleteErr }

type fakeSessionStore struct {
	session      *store.SessionWithUser
	getErr       error
	created      *store.Session
	createErr    error
	deleteErr    error
	deleteCalled bool
}

func (f *fakeSessionStore) GetByTokenHash(_ context.Context, _ string) (*store.SessionWithUser, error) {
	return f.session, f.getErr
}
func (f *fakeSessionStore) Create(_ context.Context, _, _ string) (*store.Session, error) {
	return f.created, f.createErr
}
func (f *fakeSessionStore) Delete(_ context.Context, _ string) error {
	f.deleteCalled = true
	return f.deleteErr
}
func (f *fakeSessionStore) DeleteByUserID(_ context.Context, _ string) error { return nil }

type fakeAuthCodeStore struct {
	code        string
	createErr   error
	authCode    *store.AuthCode
	exchangeErr error
}

func (f *fakeAuthCodeStore) Create(_ context.Context, _, _, _ string) (string, error) {
	return f.code, f.createErr
}
func (f *fakeAuthCodeStore) Exchange(_ context.Context, _ string) (*store.AuthCode, error) {
	return f.authCode, f.exchangeErr
}

type fakeProviderRegistry struct{}

func (f *fakeProviderRegistry) Get(_ string) (rp.RelyingParty, bool) { return nil, false }
func (f *fakeProviderRegistry) Allowed(_ string) bool                { return false }

type fakeWatchlistStore struct {
	items         []store.WatchlistItem
	listErr       error
	checkItem     *store.WatchlistItem
	checkErr      error
	createdItem   *store.WatchlistItem
	createErr     error
	updatedItem   *store.WatchlistItem
	updateErr     error
	updateSummErr error
	deleteErr     error
}

func (f *fakeWatchlistStore) List(_ context.Context, _ string, _, _ *string) ([]store.WatchlistItem, error) {
	return f.items, f.listErr
}
func (f *fakeWatchlistStore) Check(_ context.Context, _, _ string, _ int) (*store.WatchlistItem, error) {
	return f.checkItem, f.checkErr
}
func (f *fakeWatchlistStore) Create(_ context.Context, _, _ string, _ int, _ string, _ store.ResolvedMedia) (*store.WatchlistItem, error) {
	return f.createdItem, f.createErr
}
func (f *fakeWatchlistStore) UpdateStatus(_ context.Context, _, _, _ string) (*store.WatchlistItem, error) {
	return f.updatedItem, f.updateErr
}
func (f *fakeWatchlistStore) UpdateSummary(_ context.Context, _, _ string, _ int) error {
	return f.updateSummErr
}
func (f *fakeWatchlistStore) Delete(_ context.Context, _, _ string) error { return f.deleteErr }

type fakeWatchEventStore struct {
	events        []store.WatchEvent
	listErr       error
	seriesEvents  []store.WatchEvent
	seriesListErr error
	createdEvent  *store.WatchEvent
	createErr     error
	gotEvent      *store.WatchEvent
	getErr        error
	deleteErr     error
}

func (f *fakeWatchEventStore) List(_ context.Context, _ string, _ store.WatchEventFilters) ([]store.WatchEvent, error) {
	return f.events, f.listErr
}
func (f *fakeWatchEventStore) ListBySeriesID(_ context.Context, _ string, _ int) ([]store.WatchEvent, error) {
	return f.seriesEvents, f.seriesListErr
}
func (f *fakeWatchEventStore) GetByID(_ context.Context, _, _ string) (*store.WatchEvent, error) {
	return f.gotEvent, f.getErr
}
func (f *fakeWatchEventStore) Create(_ context.Context, _ store.WatchEventCreate) (*store.WatchEvent, error) {
	return f.createdEvent, f.createErr
}
func (f *fakeWatchEventStore) Delete(_ context.Context, _, _ string) error { return f.deleteErr }

type fakeAchievementStore struct {
	achievements []store.UserAchievement
	listErr      error
	unseen       []store.UserAchievement
	unseenErr    error
	unseenCount  int
	countErr     error
	markErr      error
}

func (f *fakeAchievementStore) ListByUserID(_ context.Context, _ string) ([]store.UserAchievement, error) {
	return f.achievements, f.listErr
}
func (f *fakeAchievementStore) ListUnseen(_ context.Context, _ string) ([]store.UserAchievement, error) {
	return f.unseen, f.unseenErr
}
func (f *fakeAchievementStore) CountUnseen(_ context.Context, _ string) (int, error) {
	return f.unseenCount, f.countErr
}
func (f *fakeAchievementStore) MarkAllSeen(_ context.Context, _ string) error { return f.markErr }

type fakeStatsStore struct {
	stats  *store.StatsResult
	getErr error
}

func (f *fakeStatsStore) GetStats(_ context.Context, _ string) (*store.StatsResult, error) {
	return f.stats, f.getErr
}

type fakeSeasonCastCache struct{}

func (f *fakeSeasonCastCache) Get(_ context.Context, _, _ int) (map[int]int, bool) { return nil, false }
func (f *fakeSeasonCastCache) Set(_ context.Context, _, _ int, _ map[int]int, _ time.Time) {
}

// --- TMDB test server for MetadataResolver ---

func newTMDBTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/movie/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmdb.Movie{
			ID:          550,
			Title:       "Fight Club",
			PosterPath:  "/poster.jpg",
			ReleaseDate: "1999-10-15",
			Runtime:     139,
			VoteAverage: 8.4,
			Genres:      []tmdb.Genre{{ID: 18, Name: "Drama"}},
		})
	})

	mux.HandleFunc("/tv/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmdb.Series{
			ID:             1396,
			Name:           "Breaking Bad",
			PosterPath:     "/bb.jpg",
			FirstAirDate:   "2008-01-20",
			EpisodeRunTime: []int{47},
			VoteAverage:    8.9,
			Genres:         []tmdb.Genre{{ID: 18, Name: "Drama"}},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// --- testDeps wires up all fakes ---

type testDeps struct {
	handlers         *Handlers
	mediaClient      *fakeTmdbClient
	userStore        *fakeUserStore
	sessionStore     *fakeSessionStore
	authCodeStore    *fakeAuthCodeStore
	watchlistStore   *fakeWatchlistStore
	watchEventStore  *fakeWatchEventStore
	achievementStore *fakeAchievementStore
	statsStore       *fakeStatsStore
}

func newTestHandlers(t *testing.T) *testDeps {
	t.Helper()

	tmdbSrv := newTMDBTestServer(t)
	client := tmdb.NewClient(tmdb.Config{
		BaseURL:     tmdbSrv.URL,
		Timeout:     5,
		AccessToken: "test-token",
	})
	resolver := tmdb.NewMetadataResolver(client)

	mc := &fakeTmdbClient{}
	us := &fakeUserStore{canExport: true}
	ss := &fakeSessionStore{}
	acs := &fakeAuthCodeStore{}
	ws := &fakeWatchlistStore{}
	wes := &fakeWatchEventStore{}
	as := &fakeAchievementStore{}
	sts := &fakeStatsStore{}

	h := NewHandlers(HandlerDeps{
		Cfg: &config.Config{
			MiniMovieUiSecret: "test-secret",
		},
		TmdbClient:       mc,
		TmdbResolver:     resolver,
		SeasonCastCache:  &fakeSeasonCastCache{},
		Providers:        &fakeProviderRegistry{},
		UserStore:        us,
		SessionStore:     ss,
		AuthCodeStore:    acs,
		WatchlistStore:   ws,
		WatchEventStore:  wes,
		AchievementStore: as,
		StatsStore:       sts,
	})

	return &testDeps{
		handlers:         h,
		mediaClient:      mc,
		userStore:        us,
		sessionStore:     ss,
		authCodeStore:    acs,
		watchlistStore:   ws,
		watchEventStore:  wes,
		achievementStore: as,
		statsStore:       sts,
	}
}

// --- request helpers ---

func authedRequest(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, body)
	ctx := context.WithValue(r.Context(), mw.UserContextKey, &store.User{
		ID:        "test-user-id",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	ctx = context.WithValue(ctx, mw.SessionHashContextKey, "test-session-hash")
	return r.WithContext(ctx)
}

func withChiParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func setTestUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, mw.UserContextKey, u)
}

func decodeJSON(t *testing.T, r *httptest.ResponseRecorder, v any) {
	t.Helper()
	ct := r.Header().Get("Content-Type")
	require.Contains(t, ct, "application/json")
	err := json.NewDecoder(r.Body).Decode(v)
	require.NoError(t, err)
}
