package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type MediaType string

const (
	MediaTypeMovie  MediaType = "movie"
	MediaTypeSeries MediaType = "series"
	MediaTypePerson MediaType = "person"
)

type SearchResponse struct {
	Page         int            `json:"page"`
	TotalPages   int            `json:"totalPages"`
	TotalResults int            `json:"totalResults"`
	Results      []SearchResult `json:"results"`
}

type SearchResult struct {
	ID          int       `json:"id"`
	MediaType   MediaType `json:"mediaType"`
	Title       string    `json:"title"`
	Overview    string    `json:"overview,omitempty"`
	PosterPath  string    `json:"posterPath,omitempty"`
	ReleaseDate string    `json:"releaseDate,omitempty"`
	KnownFor    string    `json:"knownFor,omitempty"`
	Age         int       `json:"age,omitempty"`
}

func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		httputil.Error(w, http.StatusBadRequest, "Query parameter 'q' is required")
		return
	}

	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	mediaType := MediaType(r.URL.Query().Get("type"))

	var results *tmdb.SearchResults
	var err error

	switch mediaType {
	case MediaTypeMovie:
		results, err = h.tmdbClient.SearchMovies(r.Context(), query, page)
	case MediaTypeSeries:
		results, err = h.tmdbClient.SearchSeries(r.Context(), query, page)
	case MediaTypePerson:
		results, err = h.tmdbClient.SearchPerson(r.Context(), query, page)
	case "", "all":
		results, err = h.tmdbClient.SearchMulti(r.Context(), query, page)
	default:
		httputil.Error(w, http.StatusBadRequest, "Invalid type parameter. Must be one of: all, movie, series, person")
		return
	}

	if err != nil {
		log.Error().Err(err).Str("query", query).Str("type", string(mediaType)).Msg("failed to search")
		httputil.Error(w, http.StatusInternalServerError, "Failed to search")
		return
	}

	refs, err := h.catalog.SeedSearch(r.Context(), results)
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("failed to seed search results")
		httputil.Error(w, http.StatusInternalServerError, "Failed to search")
		return
	}
	httputil.JSON(w, http.StatusOK, toSearchResponse(results, refs))
}

func toSearchResponse(results *tmdb.SearchResults, refs *catalog.SearchRefs) *SearchResponse {
	items := make([]SearchResult, 0, len(results.Results))
	for _, r := range results.Results {
		if item, ok := toSearchResult(r, refs); ok {
			items = append(items, item)
		}
	}

	return &SearchResponse{
		Page:         results.Page,
		TotalPages:   results.TotalPages,
		TotalResults: results.TotalResults,
		Results:      items,
	}
}

func toSearchResult(r tmdb.SearchResult, refs *catalog.SearchRefs) (SearchResult, bool) {
	result := SearchResult{Overview: r.Overview}
	var ok bool
	switch r.MediaType {
	case tmdb.MediaTypeMovie:
		result.ID, ok = refs.Movies[r.ID]
		result.MediaType = MediaTypeMovie
		result.Title = r.Title
		result.PosterPath = r.PosterPath
		result.ReleaseDate = r.ReleaseDate
	case tmdb.MediaTypeTV:
		result.ID, ok = refs.Series[r.ID]
		result.MediaType = MediaTypeSeries
		result.Title = r.Name
		result.PosterPath = r.PosterPath
		result.ReleaseDate = r.FirstAirDate
	case tmdb.MediaTypePerson:
		var d store.PersonDates
		d, ok = refs.People[r.ID]
		result.ID = d.ID
		result.Age = personAge(d)
		result.MediaType = MediaTypePerson
		result.Title = r.Name
		result.PosterPath = r.ProfilePath
		result.KnownFor = r.KnownForDepartment
	}
	return result, ok
}

func personAge(d store.PersonDates) int {
	if d.DateOfBirth == "" {
		return 0
	}
	if d.DateOfDeath != "" {
		return -1
	}
	if a := age.CalculateAge(d.DateOfBirth, time.Now().Format(time.DateOnly)); a != nil {
		return *a
	}
	return 0
}
