package catalog

import (
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestQueuedGapsTakesTheBestPrioritiesUpToTheCap(t *testing.T) {
	refs := []PersonRef{
		{SourceID: 1, Priority: PriorityCrew},
		{SourceID: 2, Priority: PriorityCast},
		{SourceID: 3, Priority: PriorityTopCast},
		{SourceID: 4, Priority: PriorityCast},
		{SourceID: 2, Priority: PriorityCrew},
	}
	known := PeopleDates{3: store.PersonDates{Fetched: true}}

	assert.Equal(t, []int{2, 4}, queuedGaps(refs, known, 2), "hydrated people are skipped, cast before crew, document order within a priority")
	assert.Equal(t, []int{2, 4, 1}, queuedGaps(refs, known, 10))
	assert.Empty(t, queuedGaps(refs, known, 0))
}
