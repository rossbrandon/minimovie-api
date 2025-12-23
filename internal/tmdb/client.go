package tmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

var (
	ErrNotFound    = errors.New("resource not found")
	ErrRateLimited = errors.New("rate limit exceeded")
	ErrServerError = errors.New("server error")
)

type Config struct {
	BaseURL     string
	Timeout     int
	AccessToken string
}

type Client struct {
	httpClient  *http.Client
	baseURL     string
	timeout     int
	accessToken string
}

func NewClient(config Config) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
		baseURL:     config.BaseURL,
		timeout:     config.Timeout,
		accessToken: config.AccessToken,
	}
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	url := c.baseURL + path
	start := time.Now()
	endpoint := extractEndpoint(path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "error", res.StatusCode, time.Since(start))
		}
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusOK:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "success", res.StatusCode, time.Since(start))
		}
	case res.StatusCode == http.StatusNotFound:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "not_found", res.StatusCode, time.Since(start))
		}
		return nil, ErrNotFound
	case res.StatusCode == http.StatusTooManyRequests:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "rate_limited", res.StatusCode, time.Since(start))
		}
		log.Warn().Msg("rate limited by TMDB: retry-after " + res.Header.Get("Retry-After"))
		return nil, ErrRateLimited
	case res.StatusCode >= 500:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "error", res.StatusCode, time.Since(start))
		}
		return nil, ErrServerError
	default:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "error", res.StatusCode, time.Since(start))
		}
		return nil, fmt.Errorf("unexpected status: %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

// extractEndpoint extracts a normalized endpoint name from a TMDB API path.
// e.g., "/movie/123" -> "movie", "/search/multi" -> "search_multi"
func extractEndpoint(path string) string {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "unknown"
	}

	switch parts[0] {
	case "movie", "tv", "person":
		return parts[0]
	case "search":
		if len(parts) > 1 {
			return "search_" + parts[1]
		}
		return "search"
	default:
		return parts[0]
	}
}
