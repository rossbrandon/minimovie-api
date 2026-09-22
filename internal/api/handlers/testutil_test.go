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
	"github.com/rossbrandon/minimovie-api/internal/background"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
)

// --- fakeTmdbClient implements tmdb.MediaClient; only search reaches it ---

type fakeTmdbClient struct {
	searchResults *tmdb.SearchResults
	searchErr     error
}

func (f *fakeTmdbClient) GetMovie(_ context.Context, _ int) (*tmdb.Movie, error)   { return nil, nil }
func (f *fakeTmdbClient) GetSeries(_ context.Context, _ int) (*tmdb.Series, error) { return nil, nil }
func (f *fakeTmdbClient) GetPerson(_ context.Context, _ int) (*tmdb.Person, error) { return nil, nil }
func (f *fakeTmdbClient) GetSeason(_ context.Context, _, _ int) (*tmdb.SeasonDetails, error) {
	return nil, nil
}
func (f *fakeTmdbClient) GetEpisode(_ context.Context, _, _, _ int) (*tmdb.EpisodeDetails, error) {
	return nil, nil
}
func (f *fakeTmdbClient) GetCollection(_ context.Context, _ int) (*tmdb.Collection, error) {
	return nil, nil
}
func (f *fakeTmdbClient) GetChanges(_ context.Context, _ tmdb.MediaType, _, _ string) ([]int, error) {
	return nil, nil
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

// --- fakeCatalog answers with the fixtures a test sets; an unset one is not found ---

type fakeCatalog struct {
	Catalog
	movie      *catalog.Movie
	series     *catalog.Series
	season     *catalog.Season
	episode    *catalog.Episode
	person     *catalog.Person
	collection *catalog.Collection
	seasons    map[int]*tmdb.SeasonDetails
	err        error
}

func (f *fakeCatalog) Movie(_ context.Context, _ int) (*catalog.Movie, error) {
	return fixture(f.movie, f.err)
}
func (f *fakeCatalog) Collection(_ context.Context, _ int) (*catalog.Collection, error) {
	return fixture(f.collection, f.err)
}
func (f *fakeCatalog) Series(_ context.Context, _ int) (*catalog.Series, error) {
	return fixture(f.series, f.err)
}
func (f *fakeCatalog) Season(_ context.Context, _, _ int) (*catalog.Season, error) {
	return fixture(f.season, f.err)
}
func (f *fakeCatalog) Seasons(_ context.Context, _ int, _ []int) (map[int]*tmdb.SeasonDetails, error) {
	return f.seasons, f.err
}
func (f *fakeCatalog) Episode(_ context.Context, _, _, _ int) (*catalog.Episode, error) {
	return fixture(f.episode, f.err)
}
func (f *fakeCatalog) Person(_ context.Context, _ int) (*catalog.Person, error) {
	return fixture(f.person, f.err)
}

// SeedSearch maps every result onto itself, as if each already had a row with the provider's id.
func (f *fakeCatalog) SeedSearch(_ context.Context, results *tmdb.SearchResults) (*catalog.SearchRefs, error) {
	refs := &catalog.SearchRefs{Movies: catalog.IDs{}, Series: catalog.IDs{}, People: catalog.PeopleDates{}}
	for _, r := range results.Results {
		switch r.MediaType {
		case tmdb.MediaTypeMovie:
			refs.Movies[r.ID] = r.ID
		case tmdb.MediaTypeTV:
			refs.Series[r.ID] = r.ID
		case tmdb.MediaTypePerson:
			refs.People[r.ID] = store.PersonDates{ID: r.ID}
		}
	}
	return refs, f.err
}

// The Resolve* snapshots are the Fight Club and Breaking Bad values the watch-event and watchlist
// tests were written against.
func (f *fakeCatalog) ResolveMovie(_ context.Context, _ int) (store.ResolvedMedia, error) {
	return store.ResolvedMedia{
		Title:          "Fight Club",
		Genres:         []string{"Drama"},
		RuntimeMinutes: ptr(139),
	}, f.err
}
func (f *fakeCatalog) ResolveSeries(_ context.Context, _ int) (store.ResolvedMedia, error) {
	return store.ResolvedMedia{Title: "Breaking Bad", Genres: []string{"Drama"}, RuntimeMinutes: ptr(47)}, f.err
}
func (f *fakeCatalog) ResolveSeason(_ context.Context, _, _ int) (store.ResolvedMedia, error) {
	return store.ResolvedMedia{Title: "Season 1", SeriesTitle: ptr("Breaking Bad"), EpisodeCount: ptr(7)}, f.err
}
func (f *fakeCatalog) ResolveEpisode(_ context.Context, _, _, _ int) (store.ResolvedMedia, error) {
	return store.ResolvedMedia{Title: "Pilot", SeriesTitle: ptr("Breaking Bad"), RuntimeMinutes: ptr(58)}, f.err
}

func fixture[T any](v *T, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, catalog.ErrNotFound
	}
	return v, nil
}

func ptr[T any](v T) *T {
	return &v
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
func (f *fakeWatchlistStore) Create(_ context.Context, _, _, _ string, _ int, _, _ string) (*store.WatchlistItem, error) {
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
func (f *fakeWatchEventStore) MarkSeason(_ context.Context, _ store.WatchEventCreate) (*store.WatchEvent, error) {
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

// --- testDeps wires up all fakes ---

type testDeps struct {
	handlers         *Handlers
	mediaClient      *fakeTmdbClient
	catalog          *fakeCatalog
	bg               *background.Group
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

	mc := &fakeTmdbClient{}
	cat := &fakeCatalog{}
	var bg background.Group
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
		Catalog:          cat,
		BG:               &bg,
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
		catalog:          cat,
		bg:               &bg,
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
