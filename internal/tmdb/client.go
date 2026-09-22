package tmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

const (
	maxAttempts = 3
	maxInFlight = 20 // max simultaneous requests across the process
)

// retryBackoff is the base wait between attempts when TMDB sends no Retry-After.
var retryBackoff = time.Second

var (
	ErrNotFound    = errors.New("resource not found")
	ErrRateLimited = errors.New("rate limit exceeded")
	ErrServerError = errors.New("server error")
	ErrTransport   = errors.New("transport error")
)

type MediaClient interface {
	GetMovie(ctx context.Context, id int) (*Movie, error)
	GetSeries(ctx context.Context, id int) (*Series, error)
	GetSeason(ctx context.Context, seriesID, seasonNumber int) (*SeasonDetails, error)
	GetEpisode(ctx context.Context, seriesID, seasonNumber, episodeNumber int) (*EpisodeDetails, error)
	GetPerson(ctx context.Context, id int) (*Person, error)
	GetCollection(ctx context.Context, id int) (*Collection, error)
	GetChanges(ctx context.Context, mediaType MediaType, startDate, endDate string) ([]int, error)
	SearchMulti(ctx context.Context, query string, page int) (*SearchResults, error)
	SearchMovies(ctx context.Context, query string, page int) (*SearchResults, error)
	SearchSeries(ctx context.Context, query string, page int) (*SearchResults, error)
	SearchPerson(ctx context.Context, query string, page int) (*SearchResults, error)
}

type Config struct {
	BaseURL     string
	Timeout     int
	AccessToken string
	RateLimit   float64 // requests per second across the process
}

type Client struct {
	httpClient  *http.Client
	baseURL     string
	accessToken string
	limiter     *rate.Limiter
	inFlight    chan struct{}

	mu        sync.Mutex
	holdUntil time.Time // set by http 429
}

func NewClient(config Config) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	c := &Client{
		httpClient: &http.Client{
			Timeout:   time.Duration(config.Timeout) * time.Second,
			Transport: transport,
		},
		baseURL:     config.BaseURL,
		accessToken: config.AccessToken,
		inFlight:    make(chan struct{}, maxInFlight),
	}
	if config.RateLimit > 0 {
		c.limiter = rate.NewLimiter(rate.Limit(config.RateLimit), 1)
	}
	return c
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return nil, err
			}
		}
		if err := c.waitForHold(ctx); err != nil {
			return nil, err
		}

		select {
		case c.inFlight <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		body, retryAfter, err := c.doGet(ctx, path)
		<-c.inFlight
		if err == nil {
			return body, nil
		}
		lastErr = err

		retryable := errors.Is(err, ErrRateLimited) || errors.Is(err, ErrServerError) ||
			(errors.Is(err, ErrTransport) && ctx.Err() == nil)
		if !retryable || attempt == maxAttempts {
			return nil, err
		}

		delay := retryAfter
		if delay <= 0 {
			delay = retryBackoff << (attempt - 1)
		}
		if errors.Is(err, ErrRateLimited) {
			if c.holdAll(delay) {
				log.Warn().Dur("pause", delay).Msg("rate limited by TMDB, pausing all requests")
			}
		} else {
			log.Debug().Str("path", path).Int("attempt", attempt).Dur("retry_in", delay).Err(err).Msg("retrying tmdb request")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

// holdAll pauses every caller until d from now.
func (c *Client) holdAll(d time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := time.Now().Add(d)
	if !until.After(c.holdUntil) {
		return false
	}
	c.holdUntil = until
	return true
}

func (c *Client) waitForHold(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.holdUntil)
	c.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

func (c *Client) doGet(ctx context.Context, path string) ([]byte, time.Duration, error) {
	url := c.baseURL + path
	start := time.Now()
	endpoint := extractEndpoint(path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "error", 0, duration)
		}
		log.Debug().Str("endpoint", endpoint).Dur("duration_ms", duration).Msg("tmdb api call failed")
		return nil, 0, fmt.Errorf("%w: %w", ErrTransport, err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusOK:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "success", res.StatusCode, duration)
		}
	case res.StatusCode == http.StatusNotFound:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "not_found", res.StatusCode, duration)
		}
		return nil, 0, ErrNotFound
	case res.StatusCode == http.StatusTooManyRequests:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "rate_limited", res.StatusCode, duration)
		}
		return nil, retryAfter(res.Header), ErrRateLimited
	case res.StatusCode >= 500:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "error", res.StatusCode, duration)
		}
		return nil, retryAfter(res.Header), ErrServerError
	default:
		if metrics.M != nil {
			metrics.M.RecordTmdbRequest(ctx, endpoint, "error", res.StatusCode, duration)
		}
		errBody, _ := io.ReadAll(res.Body)
		return nil, 0, fmt.Errorf("unexpected status: %d %s", res.StatusCode, string(errBody))
	}
	log.Debug().Str("endpoint", endpoint).Int("status", res.StatusCode).Dur("duration_ms", duration).Msg("TMDB api call completed")

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response: %w", err)
	}

	return body, 0, nil
}

// retryAfter reads a Retry-After header given in seconds. Returns 0 when absent or unparseable.
func retryAfter(h http.Header) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After")))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
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
	case "movie", "tv", "person", "collection":
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
