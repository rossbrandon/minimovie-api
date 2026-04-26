package metrics

import (
	"context"
	"time"

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

	AugurRequestsTotal   metric.Int64Counter
	AugurRequestDuration metric.Float64Histogram
	AugurFieldsTotal     metric.Int64Counter
	AugurFieldConfidence metric.Float64Histogram
	AugurTokensTotal     metric.Int64Counter
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
	m.CacheOperationsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", "write"),
		attribute.String("store", store),
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
