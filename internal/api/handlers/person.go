package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type PersonDetails struct {
	ID            int          `json:"id"`
	ImdbID        string       `json:"imdbId"`
	Name          string       `json:"name"`
	Biography     string       `json:"biography"`
	Birthday      string       `json:"birthday,omitempty"`
	Deathday      string       `json:"deathday,omitempty"`
	CurrentAge    *int         `json:"currentAge,omitempty"`
	Gender        string       `json:"gender"`
	PlaceOfBirth  string       `json:"placeOfBirth,omitempty"`
	PhotoURL      string       `json:"photoUrl"`
	KnownFor      string       `json:"knownFor"`
	AlsoKnownAs   []string     `json:"alsoKnownAs,omitempty"`
	MovieCredits  []FilmCredit `json:"movieCredits,omitempty"`
	SeriesCredits []FilmCredit `json:"seriesCredits,omitempty"`
}

type FilmCredit struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	PosterURL   string `json:"posterUrl,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Role        string `json:"role"`
	Type        string `json:"type"` // "cast" or "crew"
}

func (h *Handlers) GetPerson(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid person ID")
		return
	}

	person, err := h.tmdbClient.GetPerson(r.Context(), id)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Person not found")
			return
		}
		log.Error().Err(err).Int("person_id", id).Msg("failed to fetch person")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch person")
		return
	}

	httputil.JSON(w, http.StatusOK, toPersonDetails(person))
}

func toPersonDetails(person *tmdb.Person) *PersonDetails {
	var deathday string
	if person.Deathday != nil {
		deathday = *person.Deathday
	}

	currentAge, err := currentAge(person.Birthday, deathday)
	// Log and ignore this error so we don't break the API on bad data
	if err != nil {
		log.Error().Err(err).Str("birthday", person.Birthday).Str("deathday", deathday).Msg("failed to calculate current age")
	}

	return &PersonDetails{
		ID:            person.ID,
		ImdbID:        person.ImdbID,
		Name:          person.Name,
		Biography:     person.Biography,
		Birthday:      person.Birthday,
		Deathday:      deathday,
		CurrentAge:    currentAge,
		Gender:        genderToString(person.Gender),
		PlaceOfBirth:  person.PlaceOfBirth,
		PhotoURL:      buildImageURL(person.ProfilePath, "w92"),
		KnownFor:      person.KnownForDepartment,
		AlsoKnownAs:   person.AlsoKnownAs,
		MovieCredits:  buildFilmCredits(person.CombinedCredits, tmdb.MediaTypeMovie),
		SeriesCredits: buildFilmCredits(person.CombinedCredits, tmdb.MediaTypeTV),
	}
}

func currentAge(birthday string, deathday string) (*int, error) {
	birthDate, err := time.Parse(time.DateOnly, birthday)
	if err != nil {
		return nil, fmt.Errorf("failed to parse birthday: %w", err)
	}

	endDate := time.Now()
	if deathday != "" {
		endDate, err = time.Parse(time.DateOnly, deathday)
		if err != nil {
			return nil, fmt.Errorf("failed to parse deathday: %w", err)
		}
	}

	age := endDate.Year() - birthDate.Year()
	return &age, nil
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

func buildFilmCredits(credits tmdb.CombinedCredits, mediaType tmdb.MediaType) []FilmCredit {
	var result []FilmCredit

	// Add cast credits
	for _, c := range credits.Cast {
		if c.MediaType != string(mediaType) {
			continue
		}
		result = append(result, FilmCredit{
			ID:          c.ID,
			Title:       creditTitle(c.CombinedCreditBase, mediaType),
			PosterURL:   buildImageURL(c.PosterPath, "w92"),
			ReleaseDate: creditDate(c.CombinedCreditBase, mediaType),
			Role:        c.Character,
			Type:        "cast",
		})
	}

	// Add crew credits
	for _, c := range credits.Crew {
		if c.MediaType != string(mediaType) {
			continue
		}
		result = append(result, FilmCredit{
			ID:          c.ID,
			Title:       creditTitle(c.CombinedCreditBase, mediaType),
			PosterURL:   buildImageURL(c.PosterPath, "w92"),
			ReleaseDate: creditDate(c.CombinedCreditBase, mediaType),
			Role:        c.Job,
			Type:        "crew",
		})
	}

	// Sort by date descending (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].ReleaseDate > result[j].ReleaseDate
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
