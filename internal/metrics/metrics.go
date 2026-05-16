package metrics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const meterName = "minimovie-api"

var M *Metrics

type Metrics struct {
	HttpRequestsTotal   metric.Int64Counter
	HttpRequestDuration metric.Float64Histogram

	TmdbRequestsTotal   metric.Int64Counter
	TmdbRequestDuration metric.Float64Histogram

	DbOperationsTotal   metric.Int64Counter
	DbOperationDuration metric.Float64Histogram

	CacheOperationsTotal metric.Int64Counter
	DbRowsPurgedTotal    metric.Int64Counter

	AugurRequestsTotal       metric.Int64Counter
	AugurRequestDuration     metric.Float64Histogram
	AugurCtxRemaining        metric.Float64Histogram
	AugurFieldsTotal         metric.Int64Counter
	AugurFieldConfidence     metric.Float64Histogram
	AugurTokensTotal         metric.Int64Counter
	AuthEventsTotal          metric.Int64Counter
	WatchlistOperationsTotal metric.Int64Counter
	WatchEventsTotal         metric.Int64Counter
	AchievementsEarnedTotal  metric.Int64Counter

	SingleflightTotal     metric.Int64Counter
	BgPersistDuration     metric.Float64Histogram
	BgPersistOutcomeTotal metric.Int64Counter
	PeopleUpsertBatchSize metric.Int64Histogram
	AgeResolveFanout      metric.Int64Histogram

	DbPoolAcquiredConns        metric.Int64ObservableGauge
	DbPoolIdleConns            metric.Int64ObservableGauge
	DbPoolMaxConns             metric.Int64ObservableGauge
	DbPoolAcquireCount         metric.Int64ObservableCounter
	DbPoolCanceledAcquireCount metric.Int64ObservableCounter
}

type Config struct {
	Enabled bool
}

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		log.Info().Msg("Metrics disabled, using noop provider")
		M = initNoopMetrics()
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter,
				sdkmetric.WithInterval(30*time.Second),
			),
		),
	)
	otel.SetMeterProvider(provider)

	meter := provider.Meter(meterName)
	M, err = initMetrics(meter)
	if err != nil {
		return nil, err
	}

	log.Info().Msg("Metrics initialized")
	return provider.Shutdown, nil
}

func initMetrics(meter metric.Meter) (*Metrics, error) {
	m := &Metrics{}
	var err error

	m.HttpRequestsTotal, err = meter.Int64Counter("http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	m.HttpRequestDuration, err = meter.Float64Histogram("http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, err
	}

	m.TmdbRequestsTotal, err = meter.Int64Counter("tmdb_requests_total",
		metric.WithDescription("Total number of TMDB API requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	m.TmdbRequestDuration, err = meter.Float64Histogram("tmdb_request_duration_seconds",
		metric.WithDescription("TMDB API request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	if err != nil {
		return nil, err
	}

	m.DbOperationsTotal, err = meter.Int64Counter("db_operations_total",
		metric.WithDescription("Total number of database operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, err
	}

	m.DbOperationDuration, err = meter.Float64Histogram("db_operation_duration_seconds",
		metric.WithDescription("Database operation duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1),
	)
	if err != nil {
		return nil, err
	}

	m.CacheOperationsTotal, err = meter.Int64Counter("cache_operations_total",
		metric.WithDescription("Total number of cache operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, err
	}

	m.DbRowsPurgedTotal, err = meter.Int64Counter("db_rows_purged_total",
		metric.WithDescription("Total number of expired rows purged from cache tables"),
		metric.WithUnit("{row}"),
	)
	if err != nil {
		return nil, err
	}

	m.AugurRequestsTotal, err = meter.Int64Counter("augur_requests_total",
		metric.WithDescription("Total number of Augur LLM enrichment requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	// Expected Augur response time is 5-30s; buckets concentrate resolution in that band
	// with a short low-end runway (fast failures) and a small tail for outliers.
	m.AugurRequestDuration, err = meter.Float64Histogram("augur_request_duration_seconds",
		metric.WithDescription("Augur LLM enrichment request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(1, 2.5, 5, 7.5, 10, 12.5, 15, 20, 25, 30, 45, 60),
	)
	if err != nil {
		return nil, err
	}

	m.AugurFieldsTotal, err = meter.Int64Counter("augur_fields_total",
		metric.WithDescription("Total number of Augur fields by outcome (returned vs rejected by confidence threshold)"),
		metric.WithUnit("{field}"),
	)
	if err != nil {
		return nil, err
	}

	m.AugurFieldConfidence, err = meter.Float64Histogram("augur_field_confidence",
		metric.WithDescription("Confidence score returned by Augur for individual enriched fields (0..1)"),
		metric.WithUnit("{score}"),
		metric.WithExplicitBucketBoundaries(0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0),
	)
	if err != nil {
		return nil, err
	}

	m.AugurTokensTotal, err = meter.Int64Counter("augur_tokens_total",
		metric.WithDescription("Total tokens consumed by Augur LLM calls, split by input vs output and model"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	m.AuthEventsTotal, err = meter.Int64Counter("auth_events_total",
		metric.WithDescription("Total authentication events by provider and event type"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.WatchlistOperationsTotal, err = meter.Int64Counter("watchlist_operations_total",
		metric.WithDescription("Total watchlist mutations by operation and media type"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, err
	}

	m.WatchEventsTotal, err = meter.Int64Counter("watch_events_total",
		metric.WithDescription("Total watch events by operation and media type"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.AchievementsEarnedTotal, err = meter.Int64Counter("achievements_earned_total",
		metric.WithDescription("Total achievements awarded by achievement type"),
		metric.WithUnit("{achievement}"),
	)
	if err != nil {
		return nil, err
	}

	m.AugurCtxRemaining, err = meter.Float64Histogram("augur_ctx_remaining_seconds",
		metric.WithDescription("Context budget remaining when Augur returns; trending toward 0 means AUGUR_TIMEOUT is too tight"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0, 0.5, 1, 2, 5, 10, 15, 20, 30, 45, 60),
	)
	if err != nil {
		return nil, err
	}

	m.SingleflightTotal, err = meter.Int64Counter("singleflight_total",
		metric.WithDescription("Singleflight invocations by group and shared status; shared=true means the call piggy-backed on an in-flight call"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, err
	}

	m.BgPersistDuration, err = meter.Float64Histogram("bg_persist_duration_seconds",
		metric.WithDescription("Duration of background persist tasks (cache-warming writes detached from request context)"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	if err != nil {
		return nil, err
	}

	m.BgPersistOutcomeTotal, err = meter.Int64Counter("bg_persist_outcome_total",
		metric.WithDescription("Outcome of background persist tasks (success|error|deadline_exceeded)"),
		metric.WithUnit("{task}"),
	)
	if err != nil {
		return nil, err
	}

	m.PeopleUpsertBatchSize, err = meter.Int64Histogram("people_upsert_batch_size",
		metric.WithDescription("Number of rows per UpsertPersonBatch call"),
		metric.WithUnit("{row}"),
		metric.WithExplicitBucketBoundaries(1, 2, 5, 10, 25, 50, 100, 250, 500),
	)
	if err != nil {
		return nil, err
	}

	m.AgeResolveFanout, err = meter.Int64Histogram("age_resolve_people_count",
		metric.WithDescription("Number of people the age resolver was asked to resolve per request"),
		metric.WithUnit("{person}"),
		metric.WithExplicitBucketBoundaries(1, 2, 5, 10, 20, 50, 100),
	)
	if err != nil {
		return nil, err
	}

	m.DbPoolAcquiredConns, err = meter.Int64ObservableGauge("db_pool_acquired_conns",
		metric.WithDescription("Number of pgxpool connections currently acquired"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	m.DbPoolIdleConns, err = meter.Int64ObservableGauge("db_pool_idle_conns",
		metric.WithDescription("Number of pgxpool connections currently idle"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	m.DbPoolMaxConns, err = meter.Int64ObservableGauge("db_pool_max_conns",
		metric.WithDescription("Configured pgxpool max connections"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	m.DbPoolAcquireCount, err = meter.Int64ObservableCounter("db_pool_acquire_count_total",
		metric.WithDescription("Cumulative count of pgxpool connection acquisitions"),
		metric.WithUnit("{acquire}"),
	)
	if err != nil {
		return nil, err
	}

	m.DbPoolCanceledAcquireCount, err = meter.Int64ObservableCounter("db_pool_canceled_acquire_count_total",
		metric.WithDescription("Cumulative count of pgxpool acquisitions canceled by context (signal of pool saturation under timeout pressure)"),
		metric.WithUnit("{acquire}"),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func initNoopMetrics() *Metrics {
	provider := otel.GetMeterProvider()
	meter := provider.Meter(meterName)
	m, _ := initMetrics(meter)
	return m
}

func (m *Metrics) RecordHttpRequest(ctx context.Context, method, endpoint string, statusCode int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("endpoint", endpoint),
		attribute.Int("status_code", statusCode),
	}
	m.HttpRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.HttpRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("method", method),
		attribute.String("endpoint", endpoint),
	))
}

func (m *Metrics) RecordTmdbRequest(ctx context.Context, endpoint, status string, status_code int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("endpoint", endpoint),
		attribute.String("status", status),
		attribute.Int("status_code", status_code),
	}
	m.TmdbRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.TmdbRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("endpoint", endpoint),
	))
}

func (m *Metrics) RecordDbOperation(ctx context.Context, operation string, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
	}
	m.DbOperationsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.DbOperationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

func (m *Metrics) RecordCacheHit(ctx context.Context, store string) {
	m.CacheOperationsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", "hit"),
		attribute.String("store", store),
	))
}

func (m *Metrics) RecordCacheMiss(ctx context.Context, store string) {
	m.CacheOperationsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", "miss"),
		attribute.String("store", store),
	))
}

func (m *Metrics) RecordCacheWrite(ctx context.Context, store string) {
	m.RecordCacheWriteOutcome(ctx, store, "success")
}

func (m *Metrics) RecordCacheWriteOutcome(ctx context.Context, store, outcome string) {
	m.CacheOperationsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", "write"),
		attribute.String("store", store),
		attribute.String("outcome", outcome),
	))
}

func (m *Metrics) RecordDbPurge(ctx context.Context, table string, count int64) {
	m.DbRowsPurgedTotal.Add(ctx, count, metric.WithAttributes(
		attribute.String("table", table),
	))
}

func (m *Metrics) RecordAugurRequest(ctx context.Context, queryType, status string, duration time.Duration) {
	m.AugurRequestsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("query_type", queryType),
		attribute.String("status", status),
	))
	m.AugurRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("query_type", queryType),
	))
}

func (m *Metrics) RecordAugurField(ctx context.Context, field, outcome string, confidence float64) {
	m.AugurFieldsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("field", field),
		attribute.String("outcome", outcome),
	))
	m.AugurFieldConfidence.Record(ctx, confidence, metric.WithAttributes(
		attribute.String("field", field),
	))
}

func (m *Metrics) RecordAugurUsage(ctx context.Context, queryType, model string, inputTokens, outputTokens int64) {
	if inputTokens > 0 {
		m.AugurTokensTotal.Add(ctx, inputTokens, metric.WithAttributes(
			attribute.String("query_type", queryType),
			attribute.String("model", model),
			attribute.String("kind", "input"),
		))
	}
	if outputTokens > 0 {
		m.AugurTokensTotal.Add(ctx, outputTokens, metric.WithAttributes(
			attribute.String("query_type", queryType),
			attribute.String("model", model),
			attribute.String("kind", "output"),
		))
	}
}

func TrackDbDuration(ctx context.Context, operation string) func() {
	start := time.Now()
	return func() {
		if M != nil {
			M.RecordDbOperation(ctx, operation, time.Since(start))
		}
	}
}

func (m *Metrics) RecordAuthEvent(ctx context.Context, provider, event string) {
	m.AuthEventsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("event", event),
	))
}

func (m *Metrics) RecordWatchlistOperation(ctx context.Context, operation, mediaType string) {
	m.WatchlistOperationsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("media_type", mediaType),
	))
}

func (m *Metrics) RecordWatchEvent(ctx context.Context, operation, mediaType string) {
	m.WatchEventsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("media_type", mediaType),
	))
}

func (m *Metrics) RecordAchievementEarned(ctx context.Context, achievementID string) {
	m.AchievementsEarnedTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("achievement_id", achievementID),
	))
}

func (m *Metrics) RecordSingleflight(ctx context.Context, group string, shared bool) {
	m.SingleflightTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("group", group),
		attribute.Bool("shared", shared),
	))
}

func (m *Metrics) RecordAugurCtxRemaining(ctx context.Context, outcome string, remaining time.Duration) {
	if remaining < 0 {
		remaining = 0
	}
	m.AugurCtxRemaining.Record(ctx, remaining.Seconds(), metric.WithAttributes(
		attribute.String("outcome", outcome),
	))
}

func (m *Metrics) RecordBgPersist(ctx context.Context, task, outcome string, duration time.Duration) {
	m.BgPersistDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("task", task),
	))
	m.BgPersistOutcomeTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("task", task),
		attribute.String("outcome", outcome),
	))
}

func (m *Metrics) RecordPeopleUpsertBatchSize(ctx context.Context, size int) {
	m.PeopleUpsertBatchSize.Record(ctx, int64(size))
}

func (m *Metrics) RecordAgeResolveFanout(ctx context.Context, route string, count int) {
	m.AgeResolveFanout.Record(ctx, int64(count), metric.WithAttributes(
		attribute.String("route", route),
	))
}

func (m *Metrics) RegisterDbPoolGauges(pool *pgxpool.Pool) error {
	meter := otel.GetMeterProvider().Meter(meterName)
	_, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		stat := pool.Stat()
		o.ObserveInt64(m.DbPoolAcquiredConns, int64(stat.AcquiredConns()))
		o.ObserveInt64(m.DbPoolIdleConns, int64(stat.IdleConns()))
		o.ObserveInt64(m.DbPoolMaxConns, int64(stat.MaxConns()))
		o.ObserveInt64(m.DbPoolAcquireCount, stat.AcquireCount())
		o.ObserveInt64(m.DbPoolCanceledAcquireCount, stat.CanceledAcquireCount())
		return nil
	},
		m.DbPoolAcquiredConns,
		m.DbPoolIdleConns,
		m.DbPoolMaxConns,
		m.DbPoolAcquireCount,
		m.DbPoolCanceledAcquireCount,
	)
	return err
}
