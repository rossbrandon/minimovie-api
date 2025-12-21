package tmdb

type Movie struct {
	ID                  int                 `json:"id"`
	ImdbID              string              `json:"imdb_id"`
	Title               string              `json:"title"`
	Tagline             string              `json:"tagline"`
	Overview            string              `json:"overview"`
	Genres              []Genre             `json:"genres"`
	PosterPath          string              `json:"poster_path"`
	Status              string              `json:"status"`
	ReleaseDate         string              `json:"release_date"`
	Runtime             int                 `json:"runtime"`
	Budget              int                 `json:"budget"`
	Revenue             int                 `json:"revenue"`
	OriginalTitle       string              `json:"original_title"`
	OriginalLanguage    string              `json:"original_language"`
	OriginCountry       []string            `json:"origin_country"`
	SpokenLanguages     []SpokenLanguage    `json:"spoken_languages"`
	ProductionCompanies []ProductionCompany `json:"production_companies"`
	ProductionCountries []ProductionCountry `json:"production_countries"`
	WatchProviders      WatchProviders      `json:"watch/providers"`
	Credits             Credits             `json:"credits"`
}

type Credits struct {
	Cast []CastMember `json:"cast"`
	Crew []CrewMember `json:"crew"`
}

type CastMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
}

type CrewMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type SpokenLanguage struct {
	Name        string `json:"name"`
	EnglishName string `json:"english_name"`
	Code        string `json:"iso_639_1"`
}

type ProductionCompany struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoPath      string `json:"logo_path"`
	OriginCountry string `json:"origin_country"`
}

type ProductionCountry struct {
	Name string `json:"name"`
	Code string `json:"iso_3166_1"`
}

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
