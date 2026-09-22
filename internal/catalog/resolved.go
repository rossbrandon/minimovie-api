package catalog

import (
	"context"
	"strconv"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

func (s *Service) ResolveMovie(ctx context.Context, id int) (store.ResolvedMedia, error) {
	_, m, err := s.loadMovie(ctx, id)
	if err != nil {
		return store.ResolvedMedia{}, err
	}
	return store.ResolvedMedia{
		Title:          m.Title,
		Genres:         genreNames(m.Genres),
		RuntimeMinutes: nilIfZero(m.Runtime),
	}, nil
}

func (s *Service) ResolveSeries(ctx context.Context, id int) (store.ResolvedMedia, error) {
	_, sr, err := s.loadSeries(ctx, id)
	if err != nil {
		return store.ResolvedMedia{}, err
	}
	return store.ResolvedMedia{
		Title:          sr.Name,
		Genres:         genreNames(sr.Genres),
		RuntimeMinutes: seriesRuntime(sr),
		EpisodeCount:   nilIfZero(sr.NumberOfEpisodes),
		SeasonCount:    nilIfZero(sr.NumberOfSeasons),
	}, nil
}

func (s *Service) ResolveSeason(ctx context.Context, seriesID, seasonNumber int) (store.ResolvedMedia, error) {
	series, sr, err := s.loadSeries(ctx, seriesID)
	if err != nil {
		return store.ResolvedMedia{}, err
	}
	_, sd, err := s.loadSeason(ctx, series, seasonNumber)
	if err != nil {
		return store.ResolvedMedia{}, err
	}
	title := sd.Name
	if title == "" {
		title = sr.Name + " Season " + strconv.Itoa(seasonNumber)
	}
	return store.ResolvedMedia{
		Title:          title,
		Genres:         genreNames(sr.Genres),
		RuntimeMinutes: nilIfZero(seasonRuntime(sd)),
		SeriesTitle:    &sr.Name,
		EpisodeCount:   ptr(len(sd.Episodes)),
	}, nil
}

func (s *Service) ResolveEpisode(
	ctx context.Context,
	seriesID, seasonNumber, episodeNumber int,
) (store.ResolvedMedia, error) {
	series, sr, err := s.loadSeries(ctx, seriesID)
	if err != nil {
		return store.ResolvedMedia{}, err
	}
	_, ep, err := s.loadEpisode(ctx, series, seasonNumber, episodeNumber)
	if err != nil {
		return store.ResolvedMedia{}, err
	}
	return store.ResolvedMedia{
		Title:          ep.Name,
		Genres:         genreNames(sr.Genres),
		RuntimeMinutes: nilIfZero(ep.Runtime),
		SeriesTitle:    &sr.Name,
	}, nil
}

func seriesRuntime(sr *tmdb.Series) *int {
	if len(sr.EpisodeRunTime) == 0 || sr.EpisodeRunTime[0] <= 0 {
		return nil
	}
	perEpisode := sr.EpisodeRunTime[0]
	if sr.NumberOfEpisodes > 0 {
		return ptr(sr.NumberOfEpisodes * perEpisode)
	}
	return &perEpisode
}

func seasonRuntime(sd *tmdb.SeasonDetails) int {
	var total int
	for _, ep := range sd.Episodes {
		total += ep.Runtime
	}
	return total
}
