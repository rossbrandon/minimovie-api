package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPerson_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.person = &tmdb.Person{
		ID:                 287,
		Name:               "Brad Pitt",
		Birthday:           "1963-12-18",
		Gender:             2,
		KnownForDepartment: "Acting",
		ProfilePath:        "/path.jpg",
	}

	req := httptest.NewRequest(http.MethodGet, "/person/287", nil)
	req = withChiParams(req, map[string]string{"id": "287"})

	rr := httptest.NewRecorder()
	td.handlers.GetPerson(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp PersonDetails
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 287, resp.ID)
	assert.Equal(t, "Brad Pitt", resp.Name)
	assert.Equal(t, "Male", resp.Gender)
	require.NotNil(t, resp.CurrentAge)
	assert.Greater(t, *resp.CurrentAge, 0)
}

func TestGetPerson_InvalidID(t *testing.T) {
	td := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/person/abc", nil)
	req = withChiParams(req, map[string]string{"id": "abc"})

	rr := httptest.NewRecorder()
	td.handlers.GetPerson(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestGetPerson_NotFound(t *testing.T) {
	td := newTestHandlers(t)
	td.mediaClient.personErr = tmdb.ErrNotFound

	req := httptest.NewRequest(http.MethodGet, "/person/999999", nil)
	req = withChiParams(req, map[string]string{"id": "999999"})

	rr := httptest.NewRecorder()
	td.handlers.GetPerson(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
