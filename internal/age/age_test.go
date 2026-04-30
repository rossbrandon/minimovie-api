package age

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func intPtr(v int) *int { return &v }

func TestCalculateAge(t *testing.T) {
	tests := []struct {
		name     string
		birthday string
		date     string
		want     *int
	}{
		{"standard case", "1990-05-15", "2024-06-01", intPtr(34)},
		{"birthday not yet occurred", "1990-05-15", "2024-03-01", intPtr(33)},
		{"same day birthday", "1990-05-15", "2024-05-15", intPtr(34)},
		{"empty birthday", "", "2024-06-01", nil},
		{"empty date", "1990-05-15", "", nil},
		{"invalid date format", "1990/05/15", "2024-06-01", nil},
		{"born after date", "2025-01-01", "2024-01-01", nil},
		{"leap year birthday before leap day", "2000-02-29", "2024-02-28", intPtr(23)},
		{"leap year birthday on leap day", "2000-02-29", "2024-02-29", intPtr(24)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateAge(tt.birthday, tt.date)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestCalculateAgeAtEvent(t *testing.T) {
	tests := []struct {
		name      string
		birthday  string
		deathday  string
		eventDate string
		today     string
		want      *int
	}{
		{
			"event before death uses event date",
			"1990-05-15", "2050-01-01", "2024-06-01", "2024-06-01",
			intPtr(34),
		},
		{
			"event after death uses death date cap",
			"1990-05-15", "2020-08-01", "2024-06-01", "2024-06-01",
			intPtr(30),
		},
		{
			"person alive event in future uses today cap",
			"1990-05-15", "", "2099-06-01", "2024-06-01",
			intPtr(34),
		},
		{
			"empty birthday returns nil",
			"", "", "2024-06-01", "2024-06-01",
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateAgeAtEvent(tt.birthday, tt.deathday, tt.eventDate, tt.today)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

func TestCalculateAgeRange(t *testing.T) {
	tests := []struct {
		name      string
		birthday  string
		startDate string
		endDate   string
		want      string
	}{
		{"same start and end age", "1990-05-15", "2015-06-01", "2015-12-01", "25"},
		{"different ages", "1990-05-15", "2015-06-01", "2020-06-01", "25-30"},
		{"nil start age", "2020-01-01", "2019-06-01", "2050-06-01", "?-30"},
		{"nil end age", "1990-05-15", "2024-06-01", "", "34-"},
		{"empty birthday", "", "2024-01-01", "2024-12-31", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateAgeRange(tt.birthday, tt.startDate, tt.endDate)
			assert.Equal(t, tt.want, got)
		})
	}
}
