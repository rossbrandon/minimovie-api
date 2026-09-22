package catalog

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

func movieRow(m *tmdb.Movie, collectionID *int) (store.Movie, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return store.Movie{}, fmt.Errorf("catalog: marshal movie %d: %w", m.ID, err)
	}
	return store.Movie{
		SourceID:       m.ID,
		Title:          m.Title,
		OriginalTitle:  nilIfZero(m.OriginalTitle),
		Overview:       nilIfZero(m.Overview),
		ReleaseDate:    parseDate(m.ReleaseDate),
		RuntimeMinutes: nilIfZero(m.Runtime),
		Popularity:     m.Popularity,
		VoteAverage:    nilIfZero(m.VoteAverage),
		PosterPath:     nilIfZero(m.PosterPath),
		Genres:         genreNames(m.Genres),
		CollectionID:   collectionID,
		Payload:        payload,
	}, nil
}

func seriesRow(sr *tmdb.Series) (store.Series, error) {
	payload, err := json.Marshal(sr)
	if err != nil {
		return store.Series{}, fmt.Errorf("catalog: marshal series %d: %w", sr.ID, err)
	}
	row := store.Series{
		SourceID:      sr.ID,
		Name:          sr.Name,
		OriginalName:  nilIfZero(sr.OriginalName),
		Overview:      nilIfZero(sr.Overview),
		FirstAirDate:  parseDate(sr.FirstAirDate),
		InProduction:  ptr(sr.InProduction),
		TotalSeasons:  ptr(sr.NumberOfSeasons),
		TotalEpisodes: ptr(sr.NumberOfEpisodes),
		Popularity:    sr.Popularity,
		VoteAverage:   nilIfZero(sr.VoteAverage),
		PosterPath:    nilIfZero(sr.PosterPath),
		Genres:        genreNames(sr.Genres),
		Payload:       payload,
	}
	if sr.NextEpisodeToAir != nil {
		row.NextAirDate = parseDate(sr.NextEpisodeToAir.AirDate)
	}
	if len(sr.EpisodeRunTime) > 0 {
		row.EpisodeRunTime = nilIfZero(sr.EpisodeRunTime[0])
	}
	return row, nil
}

func seasonSkeletons(sr *tmdb.Series) []store.SeasonSkeleton {
	rows := make([]store.SeasonSkeleton, 0, len(sr.Seasons))
	for _, season := range sr.Seasons {
		rows = append(rows, store.SeasonSkeleton{
			SeasonNumber: season.SeasonNumber,
			SourceID:     season.ID,
			Name:         season.Name,
		})
	}
	return rows
}

func seasonRow(seriesID int, sd *tmdb.SeasonDetails) (store.Season, error) {
	payload, err := json.Marshal(sd)
	if err != nil {
		return store.Season{}, fmt.Errorf("catalog: marshal season %d: %w", sd.ID, err)
	}
	return store.Season{
		SeriesID:     seriesID,
		SeasonNumber: sd.SeasonNumber,
		SourceID:     sd.ID,
		Name:         sd.Name,
		Payload:      payload,
	}, nil
}

func episodeSkeletons(sd *tmdb.SeasonDetails) []store.EpisodeSkeleton {
	rows := make([]store.EpisodeSkeleton, 0, len(sd.Episodes))
	for _, ep := range sd.Episodes {
		rows = append(rows, store.EpisodeSkeleton{
			EpisodeNumber: ep.EpisodeNumber,
			SourceID:      ep.ID,
			Name:          ep.Name,
		})
	}
	return rows
}

func episodeRow(seriesID int, ep *tmdb.EpisodeDetails) (store.Episode, error) {
	payload, err := json.Marshal(ep)
	if err != nil {
		return store.Episode{}, fmt.Errorf("catalog: marshal episode %d: %w", ep.ID, err)
	}
	return store.Episode{
		SeriesID:      seriesID,
		SeasonNumber:  ep.SeasonNumber,
		EpisodeNumber: ep.EpisodeNumber,
		SourceID:      ep.ID,
		Name:          ep.Name,
		Payload:       payload,
	}, nil
}

func personRow(p *tmdb.Person) (store.Person, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return store.Person{}, fmt.Errorf("catalog: marshal person %d: %w", p.ID, err)
	}
	row := store.Person{
		SourceID:           p.ID,
		Name:               p.Name,
		DateOfBirth:        parseDate(p.Birthday),
		ProfilePath:        nilIfZero(p.ProfilePath),
		KnownForDepartment: nilIfZero(p.KnownForDepartment),
		AlsoKnownAs:        p.AlsoKnownAs,
		Popularity:         p.Popularity,
		Payload:            payload,
	}
	if p.Deathday != nil {
		row.DateOfDeath = parseDate(*p.Deathday)
	}
	return row, nil
}

func collectionRow(c *tmdb.Collection) (store.Collection, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return store.Collection{}, fmt.Errorf("catalog: marshal collection %d: %w", c.ID, err)
	}
	return store.Collection{SourceID: c.ID, Name: c.Name, Payload: payload}, nil
}

func partSkeletons(c *tmdb.Collection) []store.Skeleton {
	rows := make([]store.Skeleton, 0, len(c.Parts))
	for _, p := range c.Parts {
		rows = append(rows, store.Skeleton{
			SourceID:    p.ID,
			Title:       p.Title,
			Overview:    nilIfZero(p.Overview),
			PosterPath:  nilIfZero(p.PosterPath),
			ReleaseDate: nilIfZero(p.ReleaseDate),
			VoteAverage: nilIfZero(p.VoteAverage),
		})
	}
	return rows
}

func searchSkeletons(results *tmdb.SearchResults) (movies, series []store.Skeleton, people []store.PersonSkeleton) {
	for _, r := range results.Results {
		switch {
		case r.ID == 0:
		case r.MediaType == tmdb.MediaTypeMovie:
			movies = append(movies, store.Skeleton{
				SourceID:    r.ID,
				Title:       r.Title,
				Overview:    nilIfZero(r.Overview),
				PosterPath:  nilIfZero(r.PosterPath),
				ReleaseDate: nilIfZero(r.ReleaseDate),
				Popularity:  r.Popularity,
			})
		case r.MediaType == tmdb.MediaTypeTV:
			series = append(series, store.Skeleton{
				SourceID:    r.ID,
				Title:       r.Name,
				Overview:    nilIfZero(r.Overview),
				PosterPath:  nilIfZero(r.PosterPath),
				ReleaseDate: nilIfZero(r.FirstAirDate),
				Popularity:  r.Popularity,
			})
		case r.MediaType == tmdb.MediaTypePerson:
			people = append(people, store.PersonSkeleton{
				SourceID:           r.ID,
				Name:               r.Name,
				ProfilePath:        nilIfZero(r.ProfilePath),
				KnownForDepartment: nilIfZero(r.KnownForDepartment),
				Popularity:         r.Popularity,
			})
		}
	}
	return movies, series, people
}

func searchIDs(results *tmdb.SearchResults, mediaType tmdb.MediaType) []int {
	ids := make([]int, 0, len(results.Results))
	for _, r := range results.Results {
		if r.MediaType == mediaType && r.ID != 0 {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

func filmographyIDs(cc tmdb.CombinedCredits) (movies, series []int) {
	add := func(base tmdb.CombinedCreditBase) {
		switch base.MediaType {
		case "movie":
			movies = append(movies, base.ID)
		case "tv":
			series = append(series, base.ID)
		}
	}
	for _, c := range cc.Cast {
		add(c.CombinedCreditBase)
	}
	for _, c := range cc.Crew {
		add(c.CombinedCreditBase)
	}
	return movies, series
}

func creditSkeletons(cc tmdb.CombinedCredits) (movies, series []store.Skeleton) {
	seen := make(map[string]struct{})
	add := func(base tmdb.CombinedCreditBase, popularity float64) {
		key := base.MediaType + ":" + fmt.Sprint(base.ID)
		if _, dup := seen[key]; dup || base.ID == 0 {
			return
		}
		seen[key] = struct{}{}
		sk := store.Skeleton{
			SourceID:    base.ID,
			Popularity:  popularity,
			PosterPath:  nilIfZero(base.PosterPath),
			VoteAverage: nilIfZero(base.VoteAverage),
		}
		switch base.MediaType {
		case "movie":
			sk.Title = base.Title
			sk.ReleaseDate = nilIfZero(base.ReleaseDate)
			movies = append(movies, sk)
		case "tv":
			sk.Title = base.Name
			sk.ReleaseDate = nilIfZero(base.FirstAirDate)
			series = append(series, sk)
		}
	}
	for _, c := range cc.Cast {
		add(c.CombinedCreditBase, c.Popularity)
	}
	for _, c := range cc.Crew {
		add(c.CombinedCreditBase, c.Popularity)
	}
	return movies, series
}

func genreNames(genres []tmdb.Genre) []string {
	names := make([]string, 0, len(genres))
	for _, g := range genres {
		names = append(names, g.Name)
	}
	return names
}

func parseDate(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return nil
	}
	return &t
}

func ptr[T any](v T) *T {
	return &v
}

func nilIfZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
