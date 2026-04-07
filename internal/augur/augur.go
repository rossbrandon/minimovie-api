package augur

import (
	augurlib "github.com/rossbrandon/augur-go"
	"github.com/rossbrandon/augur-go/providers/claude"

	"github.com/rossbrandon/minimovie-api/internal/store"
)

type Config struct {
	ApiKey        string
	Model         string
	MaxTokens     int
	MaxRetries    int
	MinConfidence float64
}

type Resolver struct {
	client        *augurlib.Client
	store         *store.InterestingInfoStore
	minConfidence float64
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
func New(infoStore *store.InterestingInfoStore, cfg Config) *Resolver {
	if cfg.ApiKey == "" {
		return nil
	}

	provider := claude.NewProvider(cfg.ApiKey)

	var opts []augurlib.Option
	if cfg.Model != "" {
		opts = append(opts, augurlib.WithModel(cfg.Model))
	}
	if cfg.MaxTokens > 0 {
		opts = append(opts, augurlib.WithMaxTokens(cfg.MaxTokens))
	}
	if cfg.MaxRetries > 0 {
		opts = append(opts, augurlib.WithMaxRetries(cfg.MaxRetries))
	}

	client := augurlib.New(provider, opts...)

	return &Resolver{
		client:        client,
		store:         infoStore,
		minConfidence: cfg.MinConfidence,
	}
}
