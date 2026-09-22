package handlers

import (
	"time"

	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Person struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PhotoPath    string `json:"photoPath,omitempty"`
	Role         string `json:"role,omitempty"`
	Order        int    `json:"order,omitempty"`
	EpisodeCount int    `json:"episodeCount,omitempty"`
	Birthday     string `json:"birthday,omitempty"`
	Deathday     string `json:"deathday,omitempty"`
	CurrentAge   *int   `json:"currentAge,omitempty"`
	AgeAtRelease *int   `json:"ageAtRelease,omitempty"`
	AgeRange     string `json:"ageRange,omitempty"`
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
	case tmdb.JobDirector:
		cb.Directors = append(cb.Directors, person)
	case tmdb.JobScreenplay, tmdb.JobWriter, tmdb.JobStory:
		cb.Writers = append(cb.Writers, person)
	case tmdb.JobProducer, tmdb.JobExecutiveProducer:
		cb.Producers = append(cb.Producers, person)
	case tmdb.JobComposer:
		cb.Composers = append(cb.Composers, person)
	case tmdb.JobCinematographer:
		cb.Cinematographers = append(cb.Cinematographers, person)
	case tmdb.JobEditor:
		cb.Editors = append(cb.Editors, person)
	case tmdb.JobProductionDesign, tmdb.JobSetDesigner:
		cb.ProductionDesign = append(cb.ProductionDesign, person)
	case tmdb.JobCostumeDesign:
		cb.CostumeDesign = append(cb.CostumeDesign, person)
	case tmdb.JobCasting:
		cb.Casting = append(cb.Casting, person)
	}
}

func buildCredits(credits tmdb.Credits) *Credits {
	cast := make([]Person, len(credits.Cast))
	for i, c := range credits.Cast {
		cast[i] = Person{
			ID:        c.ID,
			Name:      c.Name,
			PhotoPath: c.ProfilePath,
			Role:      c.Character,
			Order:     c.Order,
		}
	}

	var crew crewBuilder
	for _, c := range credits.Crew {
		person := Person{
			ID:        c.ID,
			Name:      c.Name,
			PhotoPath: c.ProfilePath,
			Role:      c.Job,
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
			ID:           c.ID,
			Name:         c.Name,
			PhotoPath:    c.ProfilePath,
			Role:         character,
			Order:        c.Order,
			EpisodeCount: c.TotalEpisodeCount,
		}
	}

	var crew crewBuilder
	for _, c := range credits.Crew {
		for _, j := range c.Jobs {
			person := Person{
				ID:        c.ID,
				Name:      c.Name,
				PhotoPath: c.ProfilePath,
				Role:      j.Job,
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

// applyPeople maps every credited person onto the catalog and fills in their ages.
func applyPeople(credits *Credits, people catalog.PeopleDates, startDate, endDate string) {
	if credits == nil {
		return
	}
	buckets := []*[]Person{
		&credits.Cast, &credits.Directors, &credits.Writers, &credits.Producers, &credits.Composers,
		&credits.Cinematographers, &credits.Editors, &credits.ProductionDesign, &credits.CostumeDesign,
		&credits.Casting,
	}
	for _, bucket := range buckets {
		*bucket = mapPeople(*bucket, people, startDate, endDate)
	}
}

func mapPeople(persons []Person, people catalog.PeopleDates, startDate, endDate string) []Person {
	kept := persons[:0]
	for _, p := range persons {
		d, ok := people[p.ID]
		if !ok {
			continue
		}
		p.ID = d.ID
		kept = append(kept, withAges(p, d, startDate, endDate))
	}
	return kept
}

func withAges(p Person, d store.PersonDates, startDate, endDate string) Person {
	if d.DateOfBirth == "" {
		return p
	}
	today := time.Now().Format(time.DateOnly)
	p.Birthday = d.DateOfBirth
	p.Deathday = d.DateOfDeath
	p.CurrentAge = age.CalculateAge(d.DateOfBirth, today)
	if endDate != "" && endDate != startDate {
		p.AgeRange = age.CalculateAgeRange(d.DateOfBirth, startDate, endDate)
	} else {
		p.AgeAtRelease = age.CalculateAgeAtEvent(d.DateOfBirth, d.DateOfDeath, startDate, today)
	}
	return p
}
