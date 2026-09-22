package catalog

import (
	"context"
	"errors"
	"maps"
	"sort"
	"strconv"
	"sync"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

const (
	PriorityDirector = 1
	PriorityWriter   = 2
	PriorityTopCast  = 3
	PriorityCast     = 4
	PriorityCrew     = 5
	topCastSize      = 10
)

const (
	peopleFetcherQueue   = 1024
	peopleFetcherWorkers = 4
)

// Start runs the people fetcher, which hydrates the credited people,
func (s *Service) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.stopFetch = cancel
	s.fetchQueue = make(chan int, peopleFetcherQueue)
	for range peopleFetcherWorkers {
		s.fetchWG.Go(func() { s.fetchWorker(ctx) })
	}
}

// Stop ends the workers and waits for in-flight fetches.
func (s *Service) Stop() {
	if s.stopFetch == nil {
		return
	}
	s.stopFetch()
	s.fetchWG.Wait()
}

type PersonRef struct {
	SourceID           int
	Name               string
	ProfilePath        string
	KnownForDepartment string
	Popularity         float64
	Priority           int
}

func personRefsFromCredits(c tmdb.Credits) []PersonRef {
	refs := make([]PersonRef, 0, len(c.Cast)+len(c.Crew))
	for i, m := range c.Cast {
		refs = append(refs, PersonRef{
			SourceID:           m.ID,
			Name:               m.Name,
			ProfilePath:        m.ProfilePath,
			KnownForDepartment: m.KnownForDepartment,
			Popularity:         m.Popularity,
			Priority:           castPriority(i),
		})
	}
	for _, m := range c.Crew {
		refs = append(refs, PersonRef{
			SourceID:           m.ID,
			Name:               m.Name,
			ProfilePath:        m.ProfilePath,
			KnownForDepartment: m.KnownForDepartment,
			Popularity:         m.Popularity,
			Priority:           jobPriority(m.Job),
		})
	}
	return refs
}

func personRefsFromAggregateCredits(c tmdb.AggregateCredits) []PersonRef {
	refs := make([]PersonRef, 0, len(c.Cast)+len(c.Crew))
	for i, m := range c.Cast {
		refs = append(refs, PersonRef{
			SourceID:           m.ID,
			Name:               m.Name,
			ProfilePath:        m.ProfilePath,
			KnownForDepartment: m.KnownForDepartment,
			Popularity:         m.Popularity,
			Priority:           castPriority(i),
		})
	}
	for _, m := range c.Crew {
		priority := PriorityCrew
		for _, j := range m.Jobs {
			priority = min(priority, jobPriority(j.Job))
		}
		refs = append(refs, PersonRef{
			SourceID:           m.ID,
			Name:               m.Name,
			ProfilePath:        m.ProfilePath,
			KnownForDepartment: m.KnownForDepartment,
			Popularity:         m.Popularity,
			Priority:           priority,
		})
	}
	return refs
}

// personRefsFromSeries credits the aggregate cast and crew plus the creators.
func personRefsFromSeries(sr *tmdb.Series) []PersonRef {
	refs := personRefsFromAggregateCredits(sr.AggregateCredits)
	for _, c := range sr.CreatedBy {
		refs = append(refs, PersonRef{
			SourceID:    c.ID,
			Name:        c.Name,
			ProfilePath: c.ProfilePath,
			Priority:    PriorityCrew,
		})
	}
	return refs
}

func personRefsFromSeason(sd *tmdb.SeasonDetails) []PersonRef {
	refs := personRefsFromAggregateCredits(sd.AggregateCredits)
	for _, ep := range sd.Episodes {
		refs = append(refs, personRefsFromCast(ep.GuestStars, PriorityCast)...)
	}
	return refs
}

func personRefsFromEpisode(ep *tmdb.EpisodeDetails) []PersonRef {
	refs := personRefsFromCredits(tmdb.Credits{Cast: ep.Credits.Cast, Crew: ep.Credits.Crew})
	return append(refs, personRefsFromCast(ep.Credits.GuestStars, PriorityCast)...)
}

func personRefsFromCast(cast []tmdb.CastMember, priority int) []PersonRef {
	refs := make([]PersonRef, 0, len(cast))
	for _, m := range cast {
		refs = append(refs, PersonRef{
			SourceID:           m.ID,
			Name:               m.Name,
			ProfilePath:        m.ProfilePath,
			KnownForDepartment: m.KnownForDepartment,
			Popularity:         m.Popularity,
			Priority:           priority,
		})
	}
	return refs
}

func castPriority(order int) int {
	if order < topCastSize {
		return PriorityTopCast
	}
	return PriorityCast
}

func jobPriority(job string) int {
	switch job {
	case tmdb.JobDirector:
		return PriorityDirector
	case tmdb.JobScreenplay, tmdb.JobWriter, tmdb.JobStory:
		return PriorityWriter
	}
	return PriorityCrew
}

// seedPeople writes every credited person skeleton, so cards and later fetches have a row to start from.
func (s *Service) seedPeople(ctx context.Context, db store.DBTX, refs []PersonRef) error {
	refs = uniqueRefs(refs)
	rows := make([]store.PersonSkeleton, 0, len(refs))
	for _, r := range refs {
		rows = append(rows, store.PersonSkeleton{
			SourceID:           r.SourceID,
			Name:               r.Name,
			ProfilePath:        nilIfZero(r.ProfilePath),
			KnownForDepartment: nilIfZero(r.KnownForDepartment),
			Popularity:         r.Popularity,
		})
	}
	return s.people.UpsertSkeleton(ctx, db, rows)
}

// peopleDates maps credited people to their rows and dates. Gaps among the director, writers, and
// top cast are fetched now, up to the request cap; every other gap goes to the fetcher.
func (s *Service) peopleDates(ctx context.Context, refs []PersonRef) (PeopleDates, error) {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), missTimeout)
	defer cancel()
	known, _, err := s.fetchPriorityPeople(dctx, refs, s.maxFetchPerRequest)
	if err != nil {
		return nil, err
	}
	var gaps []int
	for _, r := range uniqueRefs(refs) {
		if !known[r.SourceID].Fetched {
			gaps = append(gaps, r.SourceID)
		}
	}
	s.enqueuePeople(gaps)
	return known, nil
}

// fetchPriorityPeople hydrates up to limit of the director, writers, and top cast that have no
// hydrated row yet. It returns what is known about every ref afterwards and how many were fetched.
func (s *Service) fetchPriorityPeople(
	ctx context.Context,
	refs []PersonRef,
	limit int,
) (map[int]store.PersonDates, int, error) {
	refs = uniqueRefs(refs)
	sourceIDs := make([]int, 0, len(refs))
	for _, r := range refs {
		sourceIDs = append(sourceIDs, r.SourceID)
	}
	known, err := s.people.GetDates(ctx, sourceIDs)
	if err != nil {
		return nil, 0, err
	}

	var missing []PersonRef
	for _, r := range refs {
		if r.Priority <= PriorityTopCast && !known[r.SourceID].Fetched {
			missing = append(missing, r)
		}
	}
	sort.SliceStable(missing, func(i, j int) bool { return missing[i].Priority < missing[j].Priority })
	if len(missing) > limit {
		missing = missing[:limit]
	}

	fetched := s.fetchPeople(ctx, missing)
	if len(fetched) > 0 {
		fresh, err := s.people.GetDates(ctx, fetched)
		if err != nil {
			return nil, 0, err
		}
		maps.Copy(known, fresh)
	}
	return known, len(fetched), ctx.Err()
}

// fetchPeople fetches refs side by side and returns the source ids that were stored.
func (s *Service) fetchPeople(ctx context.Context, refs []PersonRef) []int {
	var mu sync.Mutex
	fetched := make([]int, 0, len(refs))
	var g errgroup.Group
	g.SetLimit(personFetchConcurrency)
	for _, ref := range refs {
		g.Go(func() error {
			_, err := s.fetchPerson(ctx, s.pool, ref.SourceID)
			switch {
			case err == nil:
				mu.Lock()
				fetched = append(fetched, ref.SourceID)
				mu.Unlock()
			case !errors.Is(err, tmdb.ErrNotFound):
				log.Warn().Err(err).Int("person_source_id", ref.SourceID).Msg("catalog: person fetch failed")
			}
			return nil
		})
	}
	_ = g.Wait()
	return fetched
}

// enqueuePeople hands source ids to the fetcher without blocking; a full queue drops them.
func (s *Service) enqueuePeople(sourceIDs []int) {
	for _, id := range sourceIDs {
		select {
		case s.fetchQueue <- id:
		default:
			log.Debug().Int("person_source_id", id).Msg("catalog: person fetch dropped")
		}
	}
}

func (s *Service) fetchWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.fetchQueue:
			s.fetchQueued(id)
		}
	}
}

// fetchQueued hydrates one queued person unless a request already did.
func (s *Service) fetchQueued(sourceID int) {
	ctx, cancel := context.WithTimeout(context.Background(), missTimeout)
	defer cancel()
	known, err := s.people.GetDates(ctx, []int{sourceID})
	if err == nil && known[sourceID].Fetched {
		return
	}
	if _, err := s.fetchPerson(ctx, s.pool, sourceID); err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		log.Warn().Err(err).Int("person_source_id", sourceID).Msg("catalog: queued person fetch failed")
	}
}

// uniqueRefs drops repeated and id-less credits, keeping the first occurrence's priority.
func uniqueRefs(refs []PersonRef) []PersonRef {
	out := make([]PersonRef, 0, len(refs))
	seen := make(map[int]struct{}, len(refs))
	for _, r := range refs {
		if _, dup := seen[r.SourceID]; dup || r.SourceID == 0 {
			continue
		}
		seen[r.SourceID] = struct{}{}
		out = append(out, r)
	}
	return out
}

// getPerson does the network half of fetchPerson.
// write stores the person, seed writes the filmography skeleton.
func (s *Service) getPerson(ctx context.Context, sourceID int) (*tmdb.Person, writes, error) {
	p, err := s.tmdb.GetPerson(ctx, sourceID)
	if err != nil {
		return nil, writes{}, err
	}
	w := writes{
		write: func(ctx context.Context, db store.DBTX) error {
			row, err := personRow(p)
			if err != nil {
				return err
			}
			_, err = s.people.UpsertHydrated(ctx, db, row)
			return err
		},
		seed: func(ctx context.Context, db store.DBTX) error {
			movies, series := creditSkeletons(p.CombinedCredits)
			if err := s.movies.UpsertSkeleton(ctx, db, movies); err != nil {
				return err
			}
			return s.series.UpsertSkeleton(ctx, db, series)
		},
	}
	return p, w, nil
}

// fetchPerson fetches, stores, and returns one person by source id.
// A 404 deletes the row and returns tmdb.ErrNotFound.
func (s *Service) fetchPerson(ctx context.Context, db store.DBTX, sourceID int) (*tmdb.Person, error) {
	v, err, _ := s.sf.Do("person:"+strconv.Itoa(sourceID), func() (any, error) {
		p, w, err := s.getPerson(ctx, sourceID)
		if errors.Is(err, tmdb.ErrNotFound) {
			return nil, errors.Join(err, s.people.DeleteBySourceID(ctx, db, sourceID))
		}
		if err != nil {
			return nil, err
		}
		if err := w.apply(ctx, db); err != nil {
			return nil, err
		}
		return p, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*tmdb.Person), nil
}
