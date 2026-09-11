package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rs/zerolog/log"
)

const maxFailuresKept = 100

type state struct {
	Save    *saveRun    `json:"save,omitempty"`
	Hydrate *hydrateRun `json:"hydrate,omitempty"`
}

type runRecord struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type saveRun struct {
	runRecord
	File      string `json:"file"`
	Size      int64  `json:"size"`
	LinesDone int    `json:"lines_done"`
	Upserted  int    `json:"upserted"`
	Skipped   int    `json:"skipped"`
}

type hydrateRun struct {
	runRecord
	Limit          int       `json:"limit"`
	APIRateLimit   float64   `json:"api_rate_limit"`
	MinPopularity  float64   `json:"min_popularity"`
	Runs           int       `json:"runs"`
	ElapsedSeconds float64   `json:"elapsed_s"`
	OK             int       `json:"ok"`
	Failed         int       `json:"failed"`
	Gone           int       `json:"gone"`
	PeopleFetched  int       `json:"people_fetched"`
	Failures       []failure `json:"failures,omitempty"`
}

type failure struct {
	ID        int    `json:"id"`
	Count     int    `json:"count"`
	LastError string `json:"last_error"`
}

func statePath(dir string, entity catalog.Entity) string {
	return filepath.Join(dir, "state", entity.String()+".json")
}

func readState(dir string, entity catalog.Entity) (*state, error) {
	data, err := os.ReadFile(statePath(dir, entity))
	if errors.Is(err, os.ErrNotExist) {
		return &state{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", statePath(dir, entity), err)
	}
	return &st, nil
}

func (st *state) save(dir string, entity catalog.Entity) error {
	path := statePath(dir, entity)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (st *state) checkpoint(dir string, entity catalog.Entity) {
	if err := st.save(dir, entity); err != nil {
		log.Error().Err(err).Msg("checkpoint failed")
	}
}

// checkUnfinishedRun is the rerun guard: an unfinished save or hydrate needs an explicit --resume or --fresh.
func (st *state) checkUnfinishedRun(step string, resume, fresh bool) error {
	var record *runRecord
	switch step {
	case "save":
		if st.Save != nil {
			record = &st.Save.runRecord
		}
	case "hydrate":
		if st.Hydrate != nil {
			record = &st.Hydrate.runRecord
		}
	}
	if record == nil || !record.isUnfinished() || resume || fresh {
		return nil
	}
	return usageErrorf("%s has an unfinished run from %s; pass --resume to continue or --fresh to start over",
		step, record.StartedAt.Format(time.RFC3339))
}

func (st *state) hasUnfinishedSave() bool {
	return st.Save != nil && st.Save.isUnfinished()
}

func (st *state) hasUnfinishedHydrate() bool {
	return st.Hydrate != nil && st.Hydrate.isUnfinished()
}

func (st *state) beginSave(file string, size int64) {
	st.Save = &saveRun{runRecord: runRecord{StartedAt: time.Now()}, File: file, Size: size}
}

// beginHydrateRun counts one more run on the unfinished record, or starts a fresh record.
func (st *state) beginHydrateRun(s hydrateSettings) {
	if s.resuming {
		st.Hydrate.Runs++
		return
	}
	st.Hydrate = &hydrateRun{runRecord: runRecord{StartedAt: time.Now()}, Limit: s.limit, APIRateLimit: s.apiRateLimit, MinPopularity: s.minPopularity, Runs: 1}
}

func (r *runRecord) isUnfinished() bool {
	return r.FinishedAt == nil
}

func (r *runRecord) finish() {
	now := time.Now()
	r.FinishedAt = &now
}

// attempted is how many rows this record's runs have hydrated so far.
func (h *hydrateRun) attempted() int {
	return h.OK + h.Failed + h.Gone
}

func (h *hydrateRun) recordFailure(id int, err error) {
	for i := range h.Failures {
		if h.Failures[i].ID == id {
			h.Failures[i].Count++
			h.Failures[i].LastError = err.Error()
			return
		}
	}
	h.Failures = append(h.Failures, failure{ID: id, Count: 1, LastError: err.Error()})
	if len(h.Failures) > maxFailuresKept {
		sort.Slice(h.Failures, func(i, j int) bool { return h.Failures[i].Count > h.Failures[j].Count })
		h.Failures = h.Failures[:maxFailuresKept]
	}
}
