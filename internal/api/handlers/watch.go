package handlers

import (
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type WhereToWatch struct {
	Stream []WatchProvider `json:"stream,omitempty"`
	Rent   []WatchProvider `json:"rent,omitempty"`
	Buy    []WatchProvider `json:"buy,omitempty"`
	Free   []WatchProvider `json:"free,omitempty"`
	Ads    []WatchProvider `json:"ads,omitempty"`
}

type WatchProvider struct {
	Name     string `json:"name"`
	LogoPath string `json:"logoPath"`
}

func buildWhereToWatch(wp tmdb.WatchProviders, country string) *WhereToWatch {
	countryProviders, ok := wp.Results[country]
	if !ok {
		return nil
	}

	return &WhereToWatch{
		Stream: toWatchProviders(countryProviders.Flatrate),
		Rent:   toWatchProviders(countryProviders.Rent),
		Buy:    toWatchProviders(countryProviders.Buy),
		Free:   toWatchProviders(countryProviders.Free),
		Ads:    toWatchProviders(countryProviders.Ads),
	}
}

func toWatchProviders(providers []tmdb.Provider) []WatchProvider {
	if len(providers) == 0 {
		return nil
	}
	result := make([]WatchProvider, len(providers))
	for i, p := range providers {
		result[i] = WatchProvider{
			Name:     p.ProviderName,
			LogoPath: p.LogoPath,
		}
	}
	return result
}
