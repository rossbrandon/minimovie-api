package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, rateLimit float64) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return NewClient(Config{BaseURL: srv.URL, Timeout: 5, AccessToken: "test", RateLimit: rateLimit}), &calls
}

func TestGet_RetriesRateLimitThenSucceeds(t *testing.T) {
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = time.Second })

	var n atomic.Int32
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	}, 0)

	body, err := client.get(context.Background(), "/movie/1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1}`, string(body))
	assert.Equal(t, int32(2), calls.Load())
}

func TestGet_GivesUpAfterThreeServerErrors(t *testing.T) {
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = time.Second })

	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}, 0)

	_, err := client.get(context.Background(), "/movie/1")
	assert.ErrorIs(t, err, ErrServerError)
	assert.Equal(t, int32(maxAttempts), calls.Load())
}

func TestGet_RetriesTransportErrors(t *testing.T) {
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = time.Second })

	var n atomic.Int32
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			_ = conn.Close() // the connection drops before any response
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	}, 0)

	body, err := client.get(context.Background(), "/movie/1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1}`, string(body))
	assert.Equal(t, int32(2), calls.Load())
}

func TestGet_RateLimitPausesEveryCaller(t *testing.T) {
	retryBackoff = 60 * time.Millisecond
	t.Cleanup(func() { retryBackoff = time.Second })

	var n atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	}, 0)

	_, err := client.get(context.Background(), "/movie/1")
	require.NoError(t, err)

	start := time.Now()
	client.holdAll(retryBackoff) // as the 429 above did; a fresh caller must wait it out
	_, err = client.get(context.Background(), "/movie/2")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), retryBackoff, "a request issued during the pause waits for it")
}

func TestGetChanges_WalksWindowsAndDeduplicates(t *testing.T) {
	var windows []string
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		windows = append(windows, q.Get("start_date")+".."+q.Get("end_date"))
		// every window reports the same two ids plus one keyed on its start day; page 2 repeats page 1
		day := q.Get("start_date")[8:]
		_, _ = fmt.Fprintf(w, `{"results":[{"id":1},{"id":2},{"id":1%s}],"page":%s,"total_pages":2}`, day, q.Get("page"))
	}, 0)

	ids, err := client.GetChanges(context.Background(), MediaTypeMovie, "2026-01-01", "2026-01-30")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-01-01..2026-01-14", "2026-01-01..2026-01-14",
		"2026-01-15..2026-01-28", "2026-01-15..2026-01-28",
		"2026-01-29..2026-01-30", "2026-01-29..2026-01-30",
	}, windows, "14-day windows, two pages each")
	assert.Equal(t, int32(6), calls.Load())
	assert.Equal(t, []int{1, 2, 101, 115, 129}, ids, "deduplicated across pages and windows, first occurrence order")
}

func TestGet_DoesNotRetryNotFound(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}, 0)

	_, err := client.get(context.Background(), "/movie/1")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, int32(1), calls.Load())
}

func TestGet_CancelledContextEndsTheWait(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := client.get(ctx, "/movie/1")
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "got %v", err)
	assert.Less(t, time.Since(start), 500*time.Millisecond, "should not sit out the 1s backoff")
	assert.Equal(t, int32(1), calls.Load())
}

func TestGet_LimiterSpacesRequests(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}, 100)

	start := time.Now()
	for range 4 {
		_, err := client.get(context.Background(), "/movie/1")
		require.NoError(t, err)
	}
	// four calls at 100 req/s with a burst of one: the first is free, the next three wait 10ms each.
	assert.GreaterOrEqual(t, time.Since(start), 25*time.Millisecond)
}

func TestRetryAfter(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"absent", "", 0},
		{"seconds", "3", 3 * time.Second},
		{"zero", "0", 0},
		{"garbage", "later", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Retry-After", tc.header)
			}
			assert.Equal(t, tc.want, retryAfter(h))
		})
	}
}
