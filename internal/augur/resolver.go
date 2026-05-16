package augur

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	augur "github.com/rossbrandon/augur-go"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

const augurQueryTypePerson = "person"
const sfGroupName = "augur_person"
const bgPersistTaskName = "augur_person"
const augurCacheReadTimeout = 3 * time.Second
const augurCacheWriteTimeout = 5 * time.Second

type personInsights struct {
	NetWorth        int64    `json:"netWorth" augur:"required,desc:Estimated net worth in USD"`
	Parents         []string `json:"parents"  augur:"desc:Names of biological or adoptive parents"`
	Siblings        []string `json:"siblings" augur:"desc:Names of known siblings"`
	Children        []string `json:"children" augur:"desc:Names of known children"`
	Spouse          string   `json:"spouse"   augur:"desc:Name of current or most recent spouse or partner"`
	InterestingFact string   `json:"interestingFact" augur:"desc:One interesting fact about the person"`
}

type cachedResult struct {
	Data  *personInsights       `json:"data"`
	Meta  map[string]*FieldMeta `json:"meta,omitempty"`
	Notes string                `json:"notes,omitempty"`
}

func (r *Resolver) GetPersonInsights(ctx context.Context, personID int, name string, bypassCache bool) (*PersonInterestingInfo, error) {
	if !bypassCache {
		if cached, ok := r.readCache(ctx, personID); ok {
			return r.buildPersonInterestingInfo(cached), nil
		}
	}

	key := sfGroupName + ":" + strconv.Itoa(personID)
	v, err, shared := r.sf.Do(key, func() (any, error) {
		return r.fetchAndCache(ctx, personID, name)
	})
	if metrics.M != nil {
		metrics.M.RecordSingleflight(ctx, sfGroupName, shared)
	}
	if err != nil {
		return nil, err
	}

	cached := v.(*cachedResult)
	return r.buildPersonInterestingInfo(cached), nil
}

func (r *Resolver) readCache(ctx context.Context, personID int) (*cachedResult, bool) {
	readCtx, cancel := context.WithTimeout(ctx, augurCacheReadTimeout)
	defer cancel()

	data, _, err := r.store.Get(readCtx, "person", personID)
	if err != nil || data == nil {
		if metrics.M != nil {
			metrics.M.RecordCacheMiss(ctx, "interesting_info")
		}
		return nil, false
	}

	var cached cachedResult
	if err := json.Unmarshal(data, &cached); err != nil || cached.Data == nil {
		if metrics.M != nil {
			metrics.M.RecordCacheMiss(ctx, "interesting_info")
		}
		return nil, false
	}

	if metrics.M != nil {
		metrics.M.RecordCacheHit(ctx, "interesting_info")
	}
	log.Info().Int("person_id", personID).Msg("serving person insights from cache")
	return &cached, true
}

func (r *Resolver) fetchAndCache(ctx context.Context, personID int, name string) (*cachedResult, error) {
	log.Info().Int("person_id", personID).Str("name", name).Msg("fetching person insights from augur")

	start := time.Now()
	resp, err := augur.Query[personInsights](ctx, r.client, &augur.Request{
		Query: fmt.Sprintf("Net worth, family relationships, and one interesting fact for the actor/actress %s", name),
		Context: "Focus on USD net worth and immediate family (parents, siblings, children, spouse). " +
			"The interesting fact should be something entertaining or surprising about the person. Keep it family friendly.",
		Options: &augur.QueryOptions{
			Sources: &augur.SourceConfig{
				MaxSearches: augur.Int(2),
			},
		},
	})
	duration := time.Since(start)

	if metrics.M != nil {
		if deadline, ok := ctx.Deadline(); ok {
			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			metrics.M.RecordAugurCtxRemaining(ctx, outcome, time.Until(deadline))
		}
	}

	if err != nil {
		if metrics.M != nil {
			metrics.M.RecordAugurRequest(ctx, augurQueryTypePerson, "error", duration)
		}
		return nil, fmt.Errorf("augur query failed: %w", err)
	}

	if resp.Data == nil {
		if metrics.M != nil {
			metrics.M.RecordAugurRequest(ctx, augurQueryTypePerson, "empty", duration)
		}
		return nil, fmt.Errorf("augur returned no data for person %d (%s)", personID, name)
	}

	if metrics.M != nil {
		metrics.M.RecordAugurRequest(ctx, augurQueryTypePerson, "success", duration)
		if resp.Usage != nil {
			metrics.M.RecordAugurUsage(
				ctx,
				augurQueryTypePerson,
				resp.Model,
				int64(resp.Usage.InputTokens),
				int64(resp.Usage.OutputTokens),
			)
		}
		for fieldName, fm := range resp.Meta {
			if fm == nil {
				continue
			}
			outcome := "returned"
			if fm.Confidence < r.minConfidence {
				outcome = "rejected"
			}
			metrics.M.RecordAugurField(ctx, fieldName, outcome, fm.Confidence)
		}
	}

	meta := buildMeta(resp.Meta)
	cached := &cachedResult{
		Data:  resp.Data,
		Meta:  meta,
		Notes: resp.Notes,
	}

	jsonData, marshalErr := json.Marshal(cached)
	if marshalErr != nil {
		log.Error().Err(marshalErr).Int("person_id", personID).Msg("failed to marshal cached result")
		return cached, nil
	}

	r.persistCacheAsync(ctx, personID, name, jsonData)

	return cached, nil
}

func (r *Resolver) persistCacheAsync(ctx context.Context, personID int, name string, data json.RawMessage) {
	go func() {
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), augurCacheWriteTimeout)
		defer cancel()

		start := time.Now()
		err := r.store.Set(bgCtx, "person", personID, name, data)
		duration := time.Since(start)
		outcome := "success"
		if err != nil {
			outcome = "error"
			if errors.Is(err, context.DeadlineExceeded) {
				outcome = "deadline_exceeded"
			}
			log.Error().Err(err).Str("outcome", outcome).Int("person_id", personID).Msg("failed to persist interesting info")
		}
		if metrics.M != nil {
			metrics.M.RecordCacheWriteOutcome(bgCtx, "interesting_info", outcome)
			metrics.M.RecordBgPersist(bgCtx, bgPersistTaskName, outcome, duration)
		}
	}()
}

func buildMeta(augurMeta map[string]*augur.FieldMeta) map[string]*FieldMeta {
	if augurMeta == nil {
		return nil
	}

	meta := make(map[string]*FieldMeta, len(augurMeta))
	for field, fm := range augurMeta {
		converted := &FieldMeta{
			Confidence: fm.Confidence,
		}
		for _, src := range fm.Sources {
			converted.Sources = append(converted.Sources, Source{
				Title: src.Title,
				URL:   src.URL,
			})
		}
		meta[field] = converted
	}
	return meta
}

func (r *Resolver) buildPersonInterestingInfo(cached *cachedResult) *PersonInterestingInfo {
	info := &PersonInterestingInfo{
		Notes: cached.Notes,
	}

	if cached.Data == nil {
		info.Notes = "No data available"
		return info
	}

	info.NetWorth = r.enrichField("netWorth", cached.Data.NetWorth, cached.Meta)
	info.Parents = r.enrichField("parents", cached.Data.Parents, cached.Meta)
	info.Siblings = r.enrichField("siblings", cached.Data.Siblings, cached.Meta)
	info.Children = r.enrichField("children", cached.Data.Children, cached.Meta)
	info.Spouse = r.enrichField("spouse", cached.Data.Spouse, cached.Meta)
	info.InterestingFact = r.enrichField("interestingFact", cached.Data.InterestingFact, cached.Meta)

	if info.NetWorth == nil && info.Parents == nil && info.Siblings == nil && info.Children == nil && info.Spouse == nil && info.InterestingFact == nil {
		info.Notes = "No data available"
	}

	return info
}

func (r *Resolver) enrichField(fieldName string, value any, meta map[string]*FieldMeta) *EnrichedField {
	fm, ok := meta[fieldName]
	if !ok || fm.Confidence < r.minConfidence {
		return nil
	}

	return &EnrichedField{
		Value:      value,
		Confidence: fm.Confidence,
		Sources:    fm.Sources,
	}
}
