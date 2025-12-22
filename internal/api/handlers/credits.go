package handlers

import "github.com/rossbrandon/minimovie-api/internal/tmdb"

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

type crewBuilder struct {
	Directors        []Person
	Writers          []Person
	Producers        []Person
	Composers        []Person
	Cinematographers []Person
	Editors          []Person
	ProductionDesign []Person
	CostumeDesign    []Person
	Casting          []Person
}

func (cb *crewBuilder) add(person Person, job string) {
	switch job {
	case "Director":
		cb.Directors = append(cb.Directors, person)
	case "Screenplay", "Writer", "Story":
		cb.Writers = append(cb.Writers, person)
	case "Producer", "Executive Producer":
		cb.Producers = append(cb.Producers, person)
	case "Original Music Composer":
		cb.Composers = append(cb.Composers, person)
	case "Director of Photography":
		cb.Cinematographers = append(cb.Cinematographers, person)
	case "Editor":
		cb.Editors = append(cb.Editors, person)
	case "Production Design", "Set Designer":
		cb.ProductionDesign = append(cb.ProductionDesign, person)
	case "Costume Design":
		cb.CostumeDesign = append(cb.CostumeDesign, person)
	case "Casting":
		cb.Casting = append(cb.Casting, person)
	}
}

func buildCredits(credits tmdb.Credits) *Credits {
	cast := make([]Person, len(credits.Cast))
	for i, c := range credits.Cast {
		cast[i] = Person{
			ID:       c.ID,
			Name:     c.Name,
			PhotoURL: buildImageURL(c.ProfilePath, "w92"),
			Role:     c.Character,
			Order:    c.Order,
		}
	}

	var crew crewBuilder
	for _, c := range credits.Crew {
		person := Person{
			ID:       c.ID,
			Name:     c.Name,
			PhotoURL: buildImageURL(c.ProfilePath, "w92"),
			Role:     c.Job,
		}
		crew.add(person, c.Job)
	}

	return &Credits{
		Cast:             cast,
		Directors:        crew.Directors,
		Writers:          crew.Writers,
		Producers:        crew.Producers,
		Composers:        crew.Composers,
		Cinematographers: crew.Cinematographers,
		Editors:          crew.Editors,
		ProductionDesign: crew.ProductionDesign,
		CostumeDesign:    crew.CostumeDesign,
		Casting:          crew.Casting,
	}
}

func buildAggregateCredits(credits tmdb.AggregateCredits) *Credits {
	cast := make([]Person, len(credits.Cast))
	for i, c := range credits.Cast {
		var character string
		if len(c.Roles) > 0 {
			character = c.Roles[0].Character
		}
		cast[i] = Person{
			ID:       c.ID,
			Name:     c.Name,
			PhotoURL: buildImageURL(c.ProfilePath, "w92"),
			Role:     character,
			Order:    c.Order,
		}
	}

	var crew crewBuilder
	for _, c := range credits.Crew {
		for _, j := range c.Jobs {
			person := Person{
				ID:       c.ID,
				Name:     c.Name,
				PhotoURL: buildImageURL(c.ProfilePath, "w92"),
				Role:     j.Job,
			}
			crew.add(person, j.Job)
		}
	}

	return &Credits{
		Cast:             cast,
		Directors:        crew.Directors,
		Writers:          crew.Writers,
		Producers:        crew.Producers,
		Composers:        crew.Composers,
		Cinematographers: crew.Cinematographers,
		Editors:          crew.Editors,
		ProductionDesign: crew.ProductionDesign,
		CostumeDesign:    crew.CostumeDesign,
		Casting:          crew.Casting,
	}
}
