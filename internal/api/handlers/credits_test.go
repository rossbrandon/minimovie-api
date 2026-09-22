package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestApplyPeople(t *testing.T) {
	birthYear := time.Now().Year() - 40
	born := func(month int) string {
		return time.Date(birthYear, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	}
	people := catalog.PeopleDates{
		1: {ID: 11, DateOfBirth: born(1)},
		2: {ID: 12, DateOfBirth: "1920-01-01", DateOfDeath: "1990-06-01"},
		3: {ID: 13},
	}
	tests := []struct {
		name      string
		person    Person
		startDate string
		endDate   string
		expected  *Person
	}{
		{
			name:      "movie release",
			person:    Person{ID: 1},
			startDate: "2020-06-01",
			endDate:   "2020-06-01",
			expected:  &Person{ID: 11, Birthday: born(1), CurrentAge: ptr(40), AgeAtRelease: ptr(2020 - birthYear)},
		},
		{
			name:      "series range",
			person:    Person{ID: 1},
			startDate: "2010-06-01",
			endDate:   "2012-06-01",
			expected: &Person{
				ID: 11, Birthday: born(1), CurrentAge: ptr(40),
				AgeRange: fmt.Sprintf("%d-%d", 2010-birthYear, 2012-birthYear),
			},
		},
		{
			name:      "deceased",
			person:    Person{ID: 2},
			startDate: "2020-06-01",
			endDate:   "2020-06-01",
			expected: &Person{
				ID: 12, Birthday: "1920-01-01", Deathday: "1990-06-01",
				CurrentAge: ptr(time.Now().Year() - 1920), AgeAtRelease: ptr(70),
			},
		},
		{
			name:      "unknown birthday keeps the id only",
			person:    Person{ID: 3, Name: "Listed"},
			startDate: "2020-06-01",
			endDate:   "2020-06-01",
			expected:  &Person{ID: 13, Name: "Listed"},
		},
		{
			name:      "no row is dropped",
			person:    Person{ID: 4},
			startDate: "2020-06-01",
			endDate:   "2020-06-01",
			expected:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credits := &Credits{Cast: []Person{tt.person}, Directors: []Person{tt.person}}
			applyPeople(credits, people, tt.startDate, tt.endDate)
			if tt.expected == nil {
				assert.Empty(t, credits.Cast)
				assert.Empty(t, credits.Directors)
				return
			}
			if assert.Len(t, credits.Cast, 1) {
				got := credits.Cast[0]
				got.CurrentAge = nil // depends on today's date; checked loosely below
				expected := *tt.expected
				expected.CurrentAge = nil
				assert.Equal(t, expected, got)
			}
			if tt.expected.CurrentAge != nil && assert.NotNil(t, credits.Cast[0].CurrentAge) {
				assert.InDelta(t, *tt.expected.CurrentAge, *credits.Cast[0].CurrentAge, 1)
			}
			assert.Equal(t, credits.Cast, credits.Directors, "every bucket is mapped the same way")
		})
	}
}

func TestApplyPeople_NilCredits(t *testing.T) {
	applyPeople(nil, catalog.PeopleDates{1: store.PersonDates{ID: 1}}, "", "")
}
