package handlers

import (
	"net/http"
	"strconv"

	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type SearchResponse struct {
	Page         int            `json:"page"`
	TotalPages   int            `json:"totalPages"`
	TotalResults int            `json:"totalResults"`
	Results      []SearchResult `json:"results"`
}

type SearchResult struct {
	ID          int    `json:"id"`
	MediaType   string `json:"mediaType"`
	Title       string `json:"title"`
	Overview    string `json:"overview,omitempty"`
	PosterURL   string `json:"posterUrl,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	KnownFor    string `json:"knownFor,omitempty"`
}

func (h *Handlers) SearchMulti(w http.ResponseWriter, r *http.Request) {
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

	results, err := h.tmdbClient.SearchMulti(r.Context(), query, page)
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("failed to search")
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
		ID:        r.ID,
		MediaType: r.MediaType,
		Overview:  r.Overview,
	}

	switch r.MediaType {
	case "movie":
		result.Title = r.Title
		result.PosterURL = buildImageURL(r.PosterPath, "w185")
		result.ReleaseDate = r.ReleaseDate
	case "tv":
		result.Title = r.Name
		result.PosterURL = buildImageURL(r.PosterPath, "w185")
		result.ReleaseDate = r.FirstAirDate
	case "person":
		result.Title = r.Name
		result.PosterURL = buildImageURL(r.ProfilePath, "w185")
		result.KnownFor = r.KnownForDepartment
	}

	return result
}
