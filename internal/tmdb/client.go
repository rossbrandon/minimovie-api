package tmdb

import (
	"errors"
	"net/http"
	"time"
)

var (
	ErrNotFound = errors.New("resource not found")
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
