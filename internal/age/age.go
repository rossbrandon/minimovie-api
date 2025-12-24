package age

import (
	"fmt"
	"time"
)

// CalculateAge calculates a person's age at a given date.
// Returns nil if birthday or date is empty or invalid.
func CalculateAge(birthday, date string) *int {
	if birthday == "" || date == "" {
		return nil
	}

	bday, err := time.Parse(time.DateOnly, birthday)
	if err != nil {
		return nil
	}

	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return nil
	}

	age := d.Year() - bday.Year()

	// Adjust if birthday hasn't occurred yet in the target year
	if d.YearDay() < bday.YearDay() {
		age--
	}

	// Don't return negative ages (person born after the date)
	if age < 0 {
		return nil
	}

	return &age
}

// CalculateAgeRange calculates the age range for a person across a date span.
// Returns a string like "25-32" or just "25" if start and end are the same.
// Returns empty string if birthday is empty or invalid.
func CalculateAgeRange(birthday, startDate, endDate string) string {
	if birthday == "" {
		return ""
	}

	startAge := CalculateAge(birthday, startDate)
	endAge := CalculateAge(birthday, endDate)

	if startAge == nil && endAge == nil {
		return ""
	}

	if startAge == nil {
		return fmt.Sprintf("?-%d", *endAge)
	}

	if endAge == nil {
		return fmt.Sprintf("%d-", *startAge)
	}

	if *startAge == *endAge {
		return fmt.Sprintf("%d", *startAge)
	}

	return fmt.Sprintf("%d-%d", *startAge, *endAge)
}
