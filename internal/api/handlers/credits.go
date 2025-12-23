package handlers

import (
	"context"

	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Person struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PhotoURL     string `json:"photoUrl,omitempty"`
	Role         string `json:"role,omitempty"`
	Order        int    `json:"order,omitempty"`
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

// collectPeopleForEnrichment gathers all people from credits in priority order for birthday enrichment.
// The sliding window in the resolver will fetch them gradually over multiple requests.
func collectPeopleForEnrichment(credits *Credits) []age.PersonRef {
	if credits == nil {
		return nil
	}

	var refs []age.PersonRef

	// Priority 1: Directors
	for _, p := range credits.Directors {
		refs = append(refs, age.PersonRef{ID: p.ID, Name: p.Name, Priority: age.PriorityDirector})
	}

	// Priority 2: Writers
	for _, p := range credits.Writers {
		refs = append(refs, age.PersonRef{ID: p.ID, Name: p.Name, Priority: age.PriorityWriter})
	}

	// Priority 3-4: Cast (top 10 = priority 3, rest = priority 4)
	for i, p := range credits.Cast {
		priority := age.PriorityTopCast
		if i >= 10 {
			priority = age.PriorityCast
		}
		refs = append(refs, age.PersonRef{ID: p.ID, Name: p.Name, Priority: priority})
	}

	// Priority 5: Other crew
	addCrew := func(people []Person) {
		for _, p := range people {
			refs = append(refs, age.PersonRef{ID: p.ID, Name: p.Name, Priority: age.PriorityCrew})
		}
	}
	addCrew(credits.Producers)
	addCrew(credits.Composers)
	addCrew(credits.Cinematographers)
	addCrew(credits.Editors)
	addCrew(credits.ProductionDesign)
	addCrew(credits.CostumeDesign)
	addCrew(credits.Casting)

	return refs
}

// enrichCreditsWithAges adds age data to credits using the age resolver.
// For single-date content (movies, episodes), pass the same date for both start and end.
// For date-range content (series), pass different dates to get age ranges.
func (h *Handlers) enrichCreditsWithAges(ctx context.Context, credits *Credits, startDate, endDate string) {
	if credits == nil || h.ageResolver == nil {
		return
	}

	people := collectPeopleForEnrichment(credits)
	if len(people) == 0 {
		return
	}

	birthdays := h.ageResolver.Resolve(ctx, people)
	useRange := endDate != "" && endDate != startDate
	applyAges := func(persons []Person) {
		for i := range persons {
			dates, ok := birthdays[persons[i].ID]
			if !ok || dates.DateOfBirth == "" {
				continue
			}

			if useRange {
				persons[i].AgeRange = age.CalculateAgeRange(dates.DateOfBirth, startDate, endDate)
			} else {
				persons[i].AgeAtRelease = age.CalculateAge(dates.DateOfBirth, startDate)
			}
		}
	}

	applyAges(credits.Cast)
	applyAges(credits.Directors)
	applyAges(credits.Writers)
	applyAges(credits.Producers)
	applyAges(credits.Composers)
	applyAges(credits.Cinematographers)
	applyAges(credits.Editors)
	applyAges(credits.ProductionDesign)
	applyAges(credits.CostumeDesign)
	applyAges(credits.Casting)
}

func (h *Handlers) enrichGuestStarsWithAges(ctx context.Context, guestStars []Person, airDate string) {
	if len(guestStars) == 0 || h.ageResolver == nil {
		return
	}

	var people []age.PersonRef
	for i, p := range guestStars {
		if i >= 25 {
			break
		}
		priority := age.PriorityTopCast
		if i >= 10 {
			priority = age.PriorityCast
		}
		people = append(people, age.PersonRef{ID: p.ID, Name: p.Name, Priority: priority})
	}

	birthdays := h.ageResolver.Resolve(ctx, people)
	for i := range guestStars {
		dates, ok := birthdays[guestStars[i].ID]
		if !ok || dates.DateOfBirth == "" {
			continue
		}
		guestStars[i].AgeAtRelease = age.CalculateAge(dates.DateOfBirth, airDate)
	}
}
