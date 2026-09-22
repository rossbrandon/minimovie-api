package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

const seasonConcurrency = 5

type PeopleDates map[int]store.PersonDates

type IDs map[int]int

type Movie struct {
	*tmdb.Movie
	ID           int
	Slug         string
	CollectionID *int
	People       PeopleDates
}

type Collection struct {
	*tmdb.Collection
	ID    int
	Slug  string
	Parts IDs
}

type Series struct {
	*tmdb.Series
	ID      int
	Slug    string
	Seasons IDs
	People  PeopleDates
}

type Season struct {
	*tmdb.SeasonDetails
	ID       int
	Episodes IDs
	People   PeopleDates
}

type Episode struct {
	*tmdb.EpisodeDetails
	ID     int
	People PeopleDates
}

type Person struct {
	*tmdb.Person
	ID     int
	Slug   string
	Movies IDs
	Series IDs
}

type SearchRefs struct {
	Movies IDs
	Series IDs
	People PeopleDates
}

func (s *Service) Movie(ctx context.Context, id int) (*Movie, error) {
	row, m, err := s.loadMovie(ctx, id)
	if err != nil {
		return nil, err
	}
	people, err := s.peopleDates(ctx, personRefsFromCredits(m.Credits))
	if err != nil {
		return nil, err
	}
	return &Movie{Movie: m, ID: row.ID, Slug: row.Slug, CollectionID: row.CollectionID, People: people}, nil
}

func (s *Service) Collection(ctx context.Context, id int) (*Collection, error) {
	row, err := s.collections.Get(ctx, id)
	if err != nil || row == nil {
		return nil, missing(err)
	}
	var c tmdb.Collection
	if err := json.Unmarshal(row.Payload, &c); err != nil {
		return nil, fmt.Errorf("catalog: collection %d: %w", id, err)
	}
	ids := make([]int, 0, len(c.Parts))
	for _, p := range c.Parts {
		ids = append(ids, p.ID)
	}
	parts, err := s.movies.IDsBySource(ctx, ids)
	if err != nil {
		return nil, err
	}
	return &Collection{Collection: &c, ID: row.ID, Slug: row.Slug, Parts: parts}, nil
}

func (s *Service) Series(ctx context.Context, id int) (*Series, error) {
	row, sr, err := s.loadSeries(ctx, id)
	if err != nil {
		return nil, err
	}
	seasons, err := s.seasons.IDsBySeries(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	people, err := s.peopleDates(ctx, personRefsFromSeries(sr))
	if err != nil {
		return nil, err
	}
	return &Series{Series: sr, ID: row.ID, Slug: row.Slug, Seasons: seasons, People: people}, nil
}

func (s *Service) Season(ctx context.Context, seriesID, seasonNumber int) (*Season, error) {
	series, err := s.ensureSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	row, sd, err := s.loadSeason(ctx, series, seasonNumber)
	if err != nil {
		return nil, err
	}
	episodes, err := s.episodes.IDsBySeason(ctx, series.ID, seasonNumber)
	if err != nil {
		return nil, err
	}
	people, err := s.peopleDates(ctx, personRefsFromSeason(sd))
	if err != nil {
		return nil, err
	}
	return &Season{SeasonDetails: sd, ID: row.ID, Episodes: episodes, People: people}, nil
}

func (s *Service) Seasons(ctx context.Context, seriesID int, numbers []int) (map[int]*tmdb.SeasonDetails, error) {
	series, err := s.ensureSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	out := make(map[int]*tmdb.SeasonDetails, len(numbers))
	var g errgroup.Group
	g.SetLimit(seasonConcurrency)
	for _, n := range numbers {
		g.Go(func() error {
			_, sd, err := s.loadSeason(ctx, series, n)
			if err != nil {
				log.Warn().Err(err).Int("series_id", seriesID).Int("season_number", n).Msg("catalog: season load failed")
				return nil
			}
			mu.Lock()
			out[n] = sd
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return out, ctx.Err()
}

func (s *Service) Episode(ctx context.Context, seriesID, seasonNumber, episodeNumber int) (*Episode, error) {
	series, err := s.ensureSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	row, ep, err := s.loadEpisode(ctx, series, seasonNumber, episodeNumber)
	if err != nil {
		return nil, err
	}
	people, err := s.peopleDates(ctx, personRefsFromEpisode(ep))
	if err != nil {
		return nil, err
	}
	return &Episode{EpisodeDetails: ep, ID: row.ID, People: people}, nil
}

func (s *Service) Person(ctx context.Context, id int) (*Person, error) {
	row, p, err := s.loadPerson(ctx, id)
	if err != nil {
		return nil, err
	}
	movieIDs, seriesIDs := filmographyIDs(p.CombinedCredits)
	movies, err := s.movies.IDsBySource(ctx, movieIDs)
	if err != nil {
		return nil, err
	}
	series, err := s.series.IDsBySource(ctx, seriesIDs)
	if err != nil {
		return nil, err
	}
	return &Person{Person: p, ID: row.ID, Slug: row.Slug, Movies: movies, Series: series}, nil
}

func (s *Service) SeedSearch(ctx context.Context, results *tmdb.SearchResults) (*SearchRefs, error) {
	refs := &SearchRefs{}
	var err error
	if refs.Movies, err = s.movies.IDsBySource(ctx, searchIDs(results, tmdb.MediaTypeMovie)); err != nil {
		return nil, err
	}
	if refs.Series, err = s.series.IDsBySource(ctx, searchIDs(results, tmdb.MediaTypeTV)); err != nil {
		return nil, err
	}
	if refs.People, err = s.people.GetDates(ctx, searchIDs(results, tmdb.MediaTypePerson)); err != nil {
		return nil, err
	}
	movies, series, people := searchSkeletons(results)
	s.bg.Go(ctx, "catalog.seed", missTimeout, func(ctx context.Context) error {
		return errors.Join(
			s.movies.UpsertSkeleton(ctx, s.pool, movies),
			s.series.UpsertSkeleton(ctx, s.pool, series),
			s.people.UpsertSkeleton(ctx, s.pool, people),
		)
	})
	return refs, nil
}
