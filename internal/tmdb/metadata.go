package tmdb

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
