package tmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusOK:
		// success, continue
	case res.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case res.StatusCode == http.StatusTooManyRequests:
		log.Warn().Msg("rate limited by TMDB: retry-after " + res.Header.Get("Retry-After"))
		return nil, ErrRateLimited
	case res.StatusCode >= 500:
		return nil, ErrServerError
	default:
		return nil, fmt.Errorf("unexpected status: %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}
