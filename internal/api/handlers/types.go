package handlers

type MovieDetails struct {
	ID                  int             `json:"id"`
	ImdbID              string          `json:"imdbID"`
	Title               string          `json:"title"`
	Tagline             string          `json:"tagline"`
	Overview            string          `json:"overview"`
	Genres              []string        `json:"genres"`
	PosterURL           string          `json:"posterUrl"`
	Status              string          `json:"status"`
	ReleaseDate         string          `json:"releaseDate"`
	Runtime             int             `json:"runtime"`
	Budget              int             `json:"budget"`
	Revenue             int             `json:"revenue"`
	OriginalTitle       string          `json:"originalTitle"`
	OriginalLanguage    string          `json:"originalLanguage"`
	OriginCountry       string          `json:"originCountry"`
	SpokenLanguages     []string        `json:"spokenLanguages"`
	ProductionCompanies []string        `json:"productionCompanies"`
	ProductionCountries []string        `json:"productionCountries"`
	WatchProviders      *WatchProviders `json:"watchProviders,omitempty"`
	Credits             *Credits        `json:"credits,omitempty"`
}

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

type Credits struct {
	Cast             []Person `json:"cast,omitempty"`
	Directors        []Person `json:"directors,omitempty"`
	Writers          []Person `json:"writers,omitempty"`
	Producers        []Person `json:"producers,omitempty"`
	Composers        []Person `json:"composers,omitempty"`
	Cinematographers []Person `json:"cinematographers,omitempty"`
	Editors          []Person `json:"editors,omitempty"`
	ProductionDesign []Person `json:"productionDesign,omitempty"`
	CostumeDesign    []Person `json:"costumeDesign,omitempty"`
	Casting          []Person `json:"casting,omitempty"`
}
