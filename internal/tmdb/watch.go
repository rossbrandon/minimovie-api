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
