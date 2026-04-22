package age

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

const (
	PriorityDirector = 1
	PriorityWriter   = 2
	PriorityTopCast  = 3
	PriorityCast     = 4
	PriorityCrew     = 5

	asyncPersistTimeout = 5 * time.Second
	dbQueryTimeout      = 3 * time.Second
)

type PersonRef struct {
	ID       int
	Name     string
	Priority int
}

type Config struct {
	MaxFetchPerRequest int
}

type Resolver struct {
	cache      store.PersonCache
	personDB   *store.PersonStore
	tmdbClient *tmdb.Client
	maxFetch   int
}

func New(ctx context.Context, personDB *store.PersonStore, tmdbClient *tmdb.Client, cfg Config) (*Resolver, error) {
	cache, err := store.NewBigCacheAdapter(ctx)
	if err != nil {
		return nil, err
	}

	maxFetch := cfg.MaxFetchPerRequest
	if maxFetch <= 0 {
		maxFetch = 10
	}

	return &Resolver{
		cache:      cache,
		personDB:   personDB,
		tmdbClient: tmdbClient,
		maxFetch:   maxFetch,
	}, nil
}

func NewResolver(cache store.PersonCache, personDB *store.PersonStore, tmdbClient *tmdb.Client, maxFetch int) *Resolver {
	return &Resolver{
		cache:      cache,
		personDB:   personDB,
		tmdbClient: tmdbClient,
		maxFetch:   maxFetch,
	}
}

func (r *Resolver) Resolve(ctx context.Context, people []PersonRef) map[int]store.PersonDates {
	if len(people) == 0 {
		return make(map[int]store.PersonDates)
	}

	nameMap := buildNameMap(people)
	result, cacheMisses := r.checkCache(people)

	if len(cacheMisses) == 0 {
		return result
	}

	dbResults := r.getPeopleFromDb(ctx, cacheMisses, result)
	needFetch := r.shouldFetchFromApi(cacheMisses, dbResults)

	if len(needFetch) == 0 {
		return result
	}

	needFetch = r.prioritizeAndLimit(needFetch)
	fetched := r.fetchFromApi(ctx, needFetch)
	r.persistFetched(ctx, fetched, nameMap, result)

	return result
}

func buildNameMap(people []PersonRef) map[int]string {
	nameMap := make(map[int]string, len(people))
	for _, p := range people {
		nameMap[p.ID] = p.Name
	}
	return nameMap
}

func (r *Resolver) checkCache(people []PersonRef) (map[int]store.PersonDates, []PersonRef) {
	log.Info().Int("people_count", len(people)).Msg("checking cache for people")
	result := make(map[int]store.PersonDates)
	var misses []PersonRef

	for _, p := range people {
		dob, dod, found := r.cache.Get(p.ID)
		if found {
			result[p.ID] = store.PersonDates{
				DateOfBirth: dob,
				DateOfDeath: dod,
				Fetched:     true,
			}
		} else {
			misses = append(misses, p)
		}
	}

	log.Info().Int("hits", len(result)).Int("misses", len(misses)).Msg("checked cache for people")
	return result, misses
}

func (r *Resolver) getPeopleFromDb(ctx context.Context, misses []PersonRef, result map[int]store.PersonDates) map[int]store.PersonDates {
	start := time.Now()
	log.Info().Int("misses_count", len(misses)).Msg("getting people from database")
	ids := make([]int, len(misses))
	for i, p := range misses {
		ids[i] = p.ID
	}

	dbCtx, cancel := context.WithTimeout(ctx, dbQueryTimeout)
	defer cancel()

	dbResults, err := r.personDB.GetPeople(dbCtx, ids)
	if err != nil {
		log.Error().Err(err).Int("misses_count", len(misses)).Msg("failed to query database for people")
		return make(map[int]store.PersonDates)
	}

	for id, dates := range dbResults {
		result[id] = dates
		if dates.Fetched {
			r.cache.Set(id, dates.DateOfBirth, dates.DateOfDeath)
		}
	}

	log.Info().Dur("duration_ms", time.Since(start)).Int("db_results_count", len(dbResults)).Msg("got people from database")
	return dbResults
}

func (r *Resolver) shouldFetchFromApi(misses []PersonRef, dbResults map[int]store.PersonDates) []PersonRef {
	var shouldFetch []PersonRef
	for _, p := range misses {
		dates, inDB := dbResults[p.ID]
		if !inDB || !dates.Fetched {
			shouldFetch = append(shouldFetch, p)
		}
	}
	return shouldFetch
}

func (r *Resolver) prioritizeAndLimit(people []PersonRef) []PersonRef {
	sort.Slice(people, func(i, j int) bool {
		return people[i].Priority < people[j].Priority
	})

	if len(people) > r.maxFetch {
		return people[:r.maxFetch]
	}
	return people
}

func (r *Resolver) fetchFromApi(ctx context.Context, people []PersonRef) map[int]store.PersonDates {
	ids := make([]int, len(people))
	for i, p := range people {
		ids[i] = p.ID
	}
	log.Info().Int("people_count", len(people)).Ints("person_ids", ids).Msg("fetching people from API")
	if len(people) == 0 {
		return make(map[int]store.PersonDates)
	}

	start := time.Now()

	result := make(map[int]store.PersonDates)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Semaphore to limit concurrent API calls
	sem := make(chan struct{}, 10)

	for _, p := range people {
		wg.Add(1)
		go func(ref PersonRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			person, err := r.tmdbClient.GetPerson(ctx, ref.ID)
			if err != nil {
				log.Warn().Err(err).Int("person_id", ref.ID).Msg("failed to fetch person from API during enrichment")
				return
			}

			dates := store.PersonDates{
				DateOfBirth: person.Birthday,
				Fetched:     true,
			}
			if person.Deathday != nil {
				dates.DateOfDeath = *person.Deathday
			}

			mu.Lock()
			result[ref.ID] = dates
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	log.Info().Dur("duration_ms", time.Since(start)).Int("people_count", len(people)).Int("fetched_count", len(result)).Msg("completed batch API fetch")
	return result
}

func (r *Resolver) persistFetched(ctx context.Context, fetched map[int]store.PersonDates, nameMap map[int]string, result map[int]store.PersonDates) {
	if len(fetched) == 0 {
		return
	}

	for id, dates := range fetched {
		r.cache.Set(id, dates.DateOfBirth, dates.DateOfDeath)
		result[id] = dates
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncPersistTimeout)
		defer cancel()

		start := time.Now()
		if err := r.personDB.UpsertPersonBatch(bgCtx, fetched, nameMap); err != nil {
			log.Error().Err(err).Msg("failed to batch upsert people to database")
		}
		log.Info().Dur("duration_ms", time.Since(start)).Int("fetched_count", len(fetched)).Msg("persisted fetched people to database")
	}()
}
