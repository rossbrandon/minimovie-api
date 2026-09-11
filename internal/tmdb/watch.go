package tmdb

type WatchProviders struct {
	Results map[string]CountryProviders `json:"results"`
}

type CountryProviders struct {
	Link     string     `json:"link"`
	Flatrate []Provider `json:"flatrate"`
	Rent     []Provider `json:"rent"`
	Buy      []Provider `json:"buy"`
	Ads      []Provider `json:"ads"`
	Free     []Provider `json:"free"`
}

type Provider struct {
	LogoPath     string `json:"logo_path"`
	ProviderName string `json:"provider_name"`
}

// Prune keeps only one country's providers so a stored payload does not carry every region.
func (w *WatchProviders) Prune(country string) {
	kept, ok := w.Results[country]
	if !ok {
		w.Results = map[string]CountryProviders{}
		return
	}
	w.Results = map[string]CountryProviders{country: kept}
}
