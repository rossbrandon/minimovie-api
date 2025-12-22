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
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
	Roles       []Role `json:"roles"`
	Order       int    `json:"order"`
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
