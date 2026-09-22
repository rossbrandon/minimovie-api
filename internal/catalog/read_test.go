package catalog

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestRowMeta_State(t *testing.T) {
	now := time.Now()
	at := func(age time.Duration) *time.Time {
		ts := now.Add(-age)
		return &ts
	}
	doc := json.RawMessage(`{}`)
	tests := []struct {
		name     string
		meta     rowMeta
		expected rowState
	}{
		{name: "skeleton", meta: rowMeta{}, expected: stateMiss},
		{name: "expired", meta: rowMeta{payload: doc, fetchedAt: at(store.ExpireAfter + time.Second)}, expected: stateMiss},
		{
			name:     "expired and stale",
			meta:     rowMeta{payload: doc, fetchedAt: at(store.ExpireAfter + time.Second), stale: true},
			expected: stateMiss,
		},
		{name: "at the boundary", meta: rowMeta{payload: doc, fetchedAt: at(store.ExpireAfter)}, expected: stateFresh},
		{name: "stale", meta: rowMeta{payload: doc, fetchedAt: at(time.Hour), stale: true}, expected: stateStale},
		{name: "fresh", meta: rowMeta{payload: doc, fetchedAt: at(time.Hour)}, expected: stateFresh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.meta.state(now))
		})
	}
}
