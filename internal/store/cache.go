package store

type PersonCache interface {
	// Get retrieves cached data for a person.
	// Returns (dob, dod, true) if found, or ("", "", false) if not in cache.
	// An empty dob/dod with found=true means we know the person has no birthday data.
	Get(personID int) (dob, dod string, found bool)

	// Set stores data for a person.
	// Empty strings for dob/dod are valid (means no data available).
	Set(personID int, dob, dod string)

	SetBatch(entries map[int]PersonDates)

	Delete(personID int)
}
