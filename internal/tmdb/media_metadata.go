package tmdb

import (
	"context"
	"strconv"
	"strings"

	"github.com/rossbrandon/minimovie-api/internal/store"
)

type MetadataResolver struct {
	client MediaClient
}

func NewMetadataResolver(client MediaClient) *MetadataResolver {
	return &MetadataResolver{client: client}
}

func (r *MetadataResolver) ResolveMovie(ctx context.Context, mediaID int) (store.ResolvedMedia, error) {
	movie, err := r.client.GetMovie(ctx, mediaID)
	if err != nil {
		return store.ResolvedMedia{}, err
	}

	genres := make([]string, len(movie.Genres))
	for i, g := range movie.Genres {
		genres[i] = g.Name
	}

	var releaseYear *int
	if movie.ReleaseDate != "" {
		if y, err := strconv.Atoi(strings.Split(movie.ReleaseDate, "-")[0]); err == nil {
			releaseYear = &y
		}
	}

	var runtime *int
	if movie.Runtime > 0 {
		runtime = &movie.Runtime
	}

	var voteAvg *float32
	if movie.VoteAverage > 0 {
		v := float32(movie.VoteAverage)
		voteAvg = &v
	}

	var poster *string
	if movie.PosterPath != "" {
		poster = &movie.PosterPath
	}

	return store.ResolvedMedia{
		Title:          movie.Title,
		PosterPath:     poster,
		Genres:         genres,
		RuntimeMinutes: runtime,
		VoteAverage:    voteAvg,
		ReleaseYear:    releaseYear,
	}, nil
}

func (r *MetadataResolver) ResolveSeries(ctx context.Context, mediaID int) (store.ResolvedMedia, error) {
	series, err := r.client.GetSeries(ctx, mediaID)
	if err != nil {
		return store.ResolvedMedia{}, err
	}

	genres := make([]string, len(series.Genres))
	for i, g := range series.Genres {
		genres[i] = g.Name
	}

	var releaseYear *int
	if series.FirstAirDate != "" {
		if y, err := strconv.Atoi(strings.Split(series.FirstAirDate, "-")[0]); err == nil {
			releaseYear = &y
		}
	}

	var avgRuntime int
	if len(series.EpisodeRunTime) > 0 {
		avgRuntime = series.EpisodeRunTime[0]
	}

	var runtime *int
	if series.NumberOfEpisodes > 0 && avgRuntime > 0 {
		total := series.NumberOfEpisodes * avgRuntime
		runtime = &total
	} else if avgRuntime > 0 {
		runtime = &avgRuntime
	}

	var voteAvg *float32
	if series.VoteAverage > 0 {
		v := float32(series.VoteAverage)
		voteAvg = &v
	}

	var poster *string
	if series.PosterPath != "" {
		poster = &series.PosterPath
	}

	var episodeCount *int
	if series.NumberOfEpisodes > 0 {
		episodeCount = &series.NumberOfEpisodes
	}
	var seasonCount *int
	if series.NumberOfSeasons > 0 {
		seasonCount = &series.NumberOfSeasons
	}

	return store.ResolvedMedia{
		Title:          series.Name,
		PosterPath:     poster,
		Genres:         genres,
		RuntimeMinutes: runtime,
		VoteAverage:    voteAvg,
		ReleaseYear:    releaseYear,
		EpisodeCount:   episodeCount,
		SeasonCount:    seasonCount,
	}, nil
}

func (r *MetadataResolver) ResolveSeason(ctx context.Context, seriesID, seasonNumber int) (store.ResolvedMedia, error) {
	series, err := r.client.GetSeries(ctx, seriesID)
	if err != nil {
		return store.ResolvedMedia{}, err
	}

	season, err := r.client.GetSeason(ctx, seriesID, seasonNumber)
	if err != nil {
		return store.ResolvedMedia{}, err
	}

	genres := make([]string, len(series.Genres))
	for i, g := range series.Genres {
		genres[i] = g.Name
	}

	episodeCount := len(season.Episodes)
	var runtime *int
	if episodeCount > 0 {
		var totalRuntime int
		for _, ep := range season.Episodes {
			totalRuntime += ep.Runtime
		}
		if totalRuntime > 0 {
			runtime = &totalRuntime
		}
	}

	var poster *string
	if season.PosterPath != "" {
		poster = &season.PosterPath
	} else if series.PosterPath != "" {
		poster = &series.PosterPath
	}

	var voteAvg *float32
	if season.VoteAverage > 0 {
		v := float32(season.VoteAverage)
		voteAvg = &v
	}

	seriesTitle := series.Name
	title := season.Name
	if title == "" {
		title = series.Name + " Season " + strconv.Itoa(seasonNumber)
	}

	return store.ResolvedMedia{
		Title:          title,
		PosterPath:     poster,
		Genres:         genres,
		RuntimeMinutes: runtime,
		VoteAverage:    voteAvg,
		SeriesTitle:    &seriesTitle,
		EpisodeCount:   &episodeCount,
	}, nil
}

func (r *MetadataResolver) ResolveEpisode(ctx context.Context, seriesID, season, episode int) (store.ResolvedMedia, error) {
	ep, err := r.client.GetEpisode(ctx, seriesID, season, episode)
	if err != nil {
		return store.ResolvedMedia{}, err
	}

	series, err := r.client.GetSeries(ctx, seriesID)
	if err != nil {
		return store.ResolvedMedia{}, err
	}

	genres := make([]string, len(series.Genres))
	for i, g := range series.Genres {
		genres[i] = g.Name
	}

	var runtime *int
	if ep.Runtime > 0 {
		runtime = &ep.Runtime
	}

	var voteAvg *float32
	if ep.VoteAverage > 0 {
		v := float32(ep.VoteAverage)
		voteAvg = &v
	}

	seriesTitle := series.Name
	episodeID := ep.ID

	return store.ResolvedMedia{
		Title:          ep.Name,
		MediaID:        &episodeID,
		Genres:         genres,
		RuntimeMinutes: runtime,
		VoteAverage:    voteAvg,
		SeriesTitle:    &seriesTitle,
	}, nil
}
