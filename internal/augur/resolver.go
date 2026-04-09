package augur

import (
	"context"
	"encoding/json"
	"fmt"

	augur "github.com/rossbrandon/augur-go"
	"github.com/rs/zerolog/log"
)

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
		data, _, err := r.store.Get(ctx, "person", personID)
		if err == nil && data != nil {
			var cached cachedResult
			if err := json.Unmarshal(data, &cached); err == nil && cached.Data != nil {
				log.Info().Int("person_id", personID).Msg("serving person insights from cache")
				return r.buildPersonInterestingInfo(&cached), nil
			}
		}
	}

	log.Info().Int("person_id", personID).Str("name", name).Msg("fetching person insights from augur")

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
	if err != nil {
		return nil, fmt.Errorf("augur query failed: %w", err)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("augur returned no data for person %d (%s)", personID, name)
	}

	meta := buildMeta(resp.Meta)
	cached := cachedResult{
		Data:  resp.Data,
		Meta:  meta,
		Notes: resp.Notes,
	}

	jsonData, err := json.Marshal(cached)
	if err != nil {
		log.Error().Err(err).Int("person_id", personID).Msg("failed to marshal cached result")
	} else {
		if setErr := r.store.Set(ctx, "person", personID, name, jsonData); setErr != nil {
			log.Error().Err(setErr).Int("person_id", personID).Msg("failed to persist interesting info")
		}
	}

	return r.buildPersonInterestingInfo(&cached), nil
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
