package handlers

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type PersonDetails struct {
	ID            int          `json:"id"`
	Slug          string       `json:"slug,omitempty"`
	ImdbID        string       `json:"imdbId"`
	Name          string       `json:"name"`
	Biography     string       `json:"biography"`
	Birthday      string       `json:"birthday,omitempty"`
	Deathday      string       `json:"deathday,omitempty"`
	CurrentAge    *int         `json:"currentAge,omitempty"`
	Gender        string       `json:"gender"`
	PlaceOfBirth  string       `json:"placeOfBirth,omitempty"`
	PhotoPath     string       `json:"photoPath"`
	KnownFor      string       `json:"knownFor"`
	AlsoKnownAs   []string     `json:"alsoKnownAs,omitempty"`
	MovieCredits  []FilmCredit `json:"movieCredits,omitempty"`
	SeriesCredits []FilmCredit `json:"seriesCredits,omitempty"`
}

type FilmCredit struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	PosterPath   string  `json:"posterPath,omitempty"`
	ReleaseDate  string  `json:"releaseDate,omitempty"`
	Role         string  `json:"role"`
	Order        *int    `json:"order,omitempty"`
	Popularity   float64 `json:"popularity"`
	VoteAverage  float64 `json:"voteAverage"`
	VoteCount    int     `json:"voteCount"`
	EpisodeCount int     `json:"episodeCount,omitempty"`
	Type         string  `json:"type"`
}

func (h *Handlers) GetPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := catalog.ParseSlugID(chi.URLParam(r, "id"))
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "Invalid person ID")
		return
	}

	person, err := h.catalog.Person(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Person not found")
			return
		}
		log.Error().Err(err).Int("person_id", id).Msg("failed to fetch person")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch person")
		return
	}

	httputil.JSON(w, http.StatusOK, toPersonDetails(person))
}

func toPersonDetails(person *catalog.Person) *PersonDetails {
	var deathday string
	if person.Deathday != nil {
		deathday = *person.Deathday
	}

	currentAge := currentAge(person.Birthday, deathday)

	return &PersonDetails{
		ID:            person.ID,
		Slug:          person.Slug,
		ImdbID:        person.ImdbID,
		Name:          person.Name,
		Biography:     person.Biography,
		Birthday:      person.Birthday,
		Deathday:      deathday,
		CurrentAge:    currentAge,
		Gender:        genderToString(person.Gender),
		PlaceOfBirth:  person.PlaceOfBirth,
		PhotoPath:     person.ProfilePath,
		KnownFor:      person.KnownForDepartment,
		AlsoKnownAs:   person.AlsoKnownAs,
		MovieCredits:  buildFilmCredits(person.CombinedCredits, tmdb.MediaTypeMovie, person.MovieIDs),
		SeriesCredits: buildFilmCredits(person.CombinedCredits, tmdb.MediaTypeTV, person.SeriesIDs),
	}
}

func currentAge(birthday string, deathday string) *int {
	endDate := time.Now().Format(time.DateOnly)
	if deathday != "" {
		endDate = deathday
	}
	return age.CalculateAge(birthday, endDate)
}

func genderToString(gender int) string {
	switch gender {
	case 1:
		return "Female"
	case 2:
		return "Male"
	case 3:
		return "Non-binary"
	default:
		return "Not specified"
	}
}

func buildFilmCredits(credits tmdb.CombinedCredits, mediaType tmdb.MediaType, ids catalog.IDs) []FilmCredit {
	var result []FilmCredit

	for _, c := range credits.Cast {
		id, ok := ids[c.ID]
		if c.MediaType != string(mediaType) || !ok {
			continue
		}
		result = append(result, FilmCredit{
			ID:           id,
			Title:        creditTitle(c.CombinedCreditBase, mediaType),
			PosterPath:   c.PosterPath,
			ReleaseDate:  creditDate(c.CombinedCreditBase, mediaType),
			Role:         c.Character,
			Order:        c.Order,
			Popularity:   c.Popularity,
			VoteAverage:  c.VoteAverage,
			VoteCount:    c.VoteCount,
			EpisodeCount: c.EpisodeCount,
			Type:         "cast",
		})
	}

	for _, c := range credits.Crew {
		id, ok := ids[c.ID]
		if c.MediaType != string(mediaType) || !ok {
			continue
		}
		result = append(result, FilmCredit{
			ID:           id,
			Title:        creditTitle(c.CombinedCreditBase, mediaType),
			PosterPath:   c.PosterPath,
			ReleaseDate:  creditDate(c.CombinedCreditBase, mediaType),
			Role:         c.Job,
			Popularity:   c.Popularity,
			VoteAverage:  c.VoteAverage,
			VoteCount:    c.VoteCount,
			EpisodeCount: c.EpisodeCount,
			Type:         "crew",
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Popularity > result[j].Popularity
	})

	return result
}

func creditTitle(base tmdb.CombinedCreditBase, mediaType tmdb.MediaType) string {
	if mediaType == tmdb.MediaTypeMovie {
		return base.Title
	}
	return base.Name
}

func creditDate(base tmdb.CombinedCreditBase, mediaType tmdb.MediaType) string {
	if mediaType == tmdb.MediaTypeMovie {
		return base.ReleaseDate
	}
	return base.FirstAirDate
}
