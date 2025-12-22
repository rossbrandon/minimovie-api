package handlers

import (
	"net/http"
	"strconv"

	"github.com/rossbrandon/minimovie-api/internal/httputil"
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
	PosterURL   string    `json:"posterUrl,omitempty"`
	ReleaseDate string    `json:"releaseDate,omitempty"`
	KnownFor    string    `json:"knownFor,omitempty"`
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

	httputil.JSON(w, http.StatusOK, toSearchResponse(results))
}

func toSearchResponse(results *tmdb.SearchResults) *SearchResponse {
	items := make([]SearchResult, len(results.Results))
	for i, r := range results.Results {
		items[i] = toSearchResult(r)
	}

	return &SearchResponse{
		Page:         results.Page,
		TotalPages:   results.TotalPages,
		TotalResults: results.TotalResults,
		Results:      items,
	}
}

func toSearchResult(r tmdb.SearchResult) SearchResult {
	result := SearchResult{
		ID:       r.ID,
		Overview: r.Overview,
	}

	switch r.MediaType {
	case tmdb.MediaTypeMovie:
		result.MediaType = MediaTypeMovie
		result.Title = r.Title
		result.PosterURL = buildImageURL(r.PosterPath, "w185")
		result.ReleaseDate = r.ReleaseDate
	case tmdb.MediaTypeTV:
		result.MediaType = MediaTypeSeries
		result.Title = r.Name
		result.PosterURL = buildImageURL(r.PosterPath, "w185")
		result.ReleaseDate = r.FirstAirDate
	case tmdb.MediaTypePerson:
		result.MediaType = MediaTypePerson
		result.Title = r.Name
		result.PosterURL = buildImageURL(r.ProfilePath, "w185")
		result.KnownFor = r.KnownForDepartment
	}

	return result
}
