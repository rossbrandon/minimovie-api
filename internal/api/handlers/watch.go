package handlers

import (
	"fmt"

	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Person struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	PhotoURL string `json:"photoUrl,omitempty"`
	Role     string `json:"role,omitempty"`
	Order    int    `json:"order,omitempty"`
}

type WatchProviders struct {
	Stream []WatchProvider `json:"stream,omitempty"`
	Rent   []WatchProvider `json:"rent,omitempty"`
	Buy    []WatchProvider `json:"buy,omitempty"`
	Free   []WatchProvider `json:"free,omitempty"`
	Ads    []WatchProvider `json:"ads,omitempty"`
}

type WatchProvider struct {
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl"`
}

func buildImageURL(path string, size string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", size, path)
}

func buildWatchProviders(wp tmdb.WatchProviders, country string) *WatchProviders {
	countryProviders, ok := wp.Results[country]
	if !ok {
		return nil
	}

	return &WatchProviders{
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
			Name:    p.ProviderName,
			LogoURL: buildImageURL(p.LogoPath, "w45"),
		}
	}
	return result
}
