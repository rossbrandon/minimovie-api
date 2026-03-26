package tmdb

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

type AggregateCredits struct {
	Cast []AggregateCastMember `json:"cast"`
	Crew []AggregateCrewMember `json:"crew"`
}

type AggregateCastMember struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	ProfilePath       string `json:"profile_path"`
	Roles             []Role `json:"roles"`
	Order             int    `json:"order"`
	TotalEpisodeCount int    `json:"total_episode_count"`
}

type Role struct {
	Character    string `json:"character"`
	EpisodeCount int    `json:"episode_count"`
}

type AggregateCrewMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
	Department  string `json:"department"`
	Jobs        []Job  `json:"jobs"`
}

type Job struct {
	Job          string `json:"job"`
	EpisodeCount int    `json:"episode_count"`
}

// CombinedCredits represents a person's filmography (movies/shows they worked on).
// This is the inverse of Credits which lists people who worked on a movie/show.
type CombinedCredits struct {
	Cast []CombinedCastCredit `json:"cast"`
	Crew []CombinedCrewCredit `json:"crew"`
}

type CombinedCreditMovie struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
}

type CombinedCreditShow struct {
	Name         string `json:"name"`
	FirstAirDate string `json:"first_air_date"`
}

// CombinedCreditBase contains fields shared by both cast and crew credits.
type CombinedCreditBase struct {
	ID          int     `json:"id"`
	MediaType   string  `json:"media_type"`
	PosterPath  string  `json:"poster_path"`
	VoteAverage float64 `json:"vote_average"`
	CombinedCreditMovie
	CombinedCreditShow
}

type CombinedCastCredit struct {
	CombinedCreditBase
	Character    string  `json:"character"`
	EpisodeCount int     `json:"episode_count"`
	Order        int     `json:"order"`
	Popularity   float64 `json:"popularity"`
}

type CombinedCrewCredit struct {
	CombinedCreditBase
	Job          string  `json:"job"`
	Department   string  `json:"department"`
	EpisodeCount int     `json:"episode_count"`
	Popularity   float64 `json:"popularity"`
}
