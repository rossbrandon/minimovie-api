package augur

import (
	"context"
	"encoding/json"
	"time"

	augur "github.com/rossbrandon/augur-go"
	"github.com/rossbrandon/augur-go/providers/claude"
	"golang.org/x/sync/singleflight"
)

type Config struct {
	ApiKey        string
	Model         string
	MaxTokens     int
	MaxRetries    int
	MinConfidence float64
}

type insightsStore interface {
	GetInsights(ctx context.Context, id int) (json.RawMessage, *time.Time, error)
	SetInsights(ctx context.Context, id int, data json.RawMessage) error
}

type Resolver struct {
	client        *augur.Client
	store         insightsStore
	minConfidence float64
	sf            singleflight.Group
}

type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type FieldMeta struct {
	Confidence float64  `json:"confidence"`
	Sources    []Source `json:"sources,omitempty"`
}

type EnrichedField struct {
	Value      any      `json:"value"`
	Confidence float64  `json:"confidence"`
	Sources    []Source `json:"sources,omitempty"`
}

type PersonInterestingInfo struct {
	NetWorth        *EnrichedField `json:"netWorth,omitempty"`
	Parents         *EnrichedField `json:"parents,omitempty"`
	Siblings        *EnrichedField `json:"siblings,omitempty"`
	Children        *EnrichedField `json:"children,omitempty"`
	Spouse          *EnrichedField `json:"spouse,omitempty"`
	InterestingFact *EnrichedField `json:"interestingFact,omitempty"`
	Notes           string         `json:"notes"`
}

// New creates a new augur Resolver. Returns nil if cfg.ApiKey is empty (feature disabled).
func New(infoStore insightsStore, cfg Config) *Resolver {
	if cfg.ApiKey == "" {
		return nil
	}

	provider := claude.NewProvider(cfg.ApiKey)

	var opts []augur.Option
	if cfg.Model != "" {
		opts = append(opts, augur.WithModel(cfg.Model))
	}
	if cfg.MaxTokens > 0 {
		opts = append(opts, augur.WithMaxTokens(cfg.MaxTokens))
	}
	if cfg.MaxRetries > 0 {
		opts = append(opts, augur.WithMaxRetries(cfg.MaxRetries))
	}

	client := augur.New(provider, opts...)

	return &Resolver{
		client:        client,
		store:         infoStore,
		minConfidence: cfg.MinConfidence,
	}
}
