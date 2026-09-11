package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unfinishedHydrate() *state {
	return &state{Hydrate: &hydrateRun{runRecord: runRecord{StartedAt: time.Now()}, Limit: 500, APIRateLimit: 5, MinPopularity: 1, Runs: 2}}
}

func TestState_RoundTripAndFailureLedger(t *testing.T) {
	dir := t.TempDir()
	st, err := readState(dir, catalog.EntityMovie)
	require.NoError(t, err)
	assert.Nil(t, st.Save)

	st.beginHydrateRun(hydrateSettings{limit: 10, apiRateLimit: 40})
	st.Hydrate.recordFailure(7, errors.New("boom"))
	st.Hydrate.recordFailure(7, errors.New("boom again"))
	st.Hydrate.recordFailure(8, errors.New("other"))
	require.NoError(t, st.save(dir, catalog.EntityMovie))

	again, err := readState(dir, catalog.EntityMovie)
	require.NoError(t, err)
	require.NotNil(t, again.Hydrate)
	assert.Equal(t, 10, again.Hydrate.Limit)
	assert.Equal(t, []failure{{ID: 7, Count: 2, LastError: "boom again"}, {ID: 8, Count: 1, LastError: "other"}}, again.Hydrate.Failures)

	_, err = os.Stat(filepath.Join(dir, "state", "movies.json.tmp"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "the temp file is renamed away")
	_, err = os.Stat(filepath.Join(dir, "state", "movies.json"))
	assert.NoError(t, err, "state files are named after the entity")
}

func TestState_CheckUnfinishedRun(t *testing.T) {
	finished := time.Now()
	cases := []struct {
		name          string
		st            *state
		resume, fresh bool
		wantErr       bool
	}{
		{"no record", &state{}, false, false, false},
		{"finished record", &state{Hydrate: &hydrateRun{runRecord: runRecord{StartedAt: finished, FinishedAt: &finished}}}, false, false, false},
		{"unfinished needs a flag", unfinishedHydrate(), false, false, true},
		{"unfinished with resume", unfinishedHydrate(), true, false, false},
		{"unfinished with fresh", unfinishedHydrate(), false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.st.checkUnfinishedRun("hydrate", tc.resume, tc.fresh)
			if tc.wantErr {
				assert.ErrorIs(t, err, errUsage)
				assert.Contains(t, err.Error(), "--resume")
				return
			}
			assert.NoError(t, err)
		})
	}
	assert.NoError(t, (&state{}).checkUnfinishedRun("save", false, false))
}

func TestResolveHydrateSettings(t *testing.T) {
	dir := t.TempDir()
	movies := entityFlag{entity: catalog.EntityMovie}

	_, got, err := resolveHydrateSettings(hydrateOptions{dir: dir, entity: movies, limit: 10, apiRateLimit: 40})
	require.NoError(t, err)
	assert.Equal(t, hydrateSettings{limit: 10, apiRateLimit: 40}, got)

	_, _, err = resolveHydrateSettings(hydrateOptions{dir: dir, entity: movies, apiRateLimit: 40})
	assert.ErrorIs(t, err, errUsage, "--limit must be positive")

	st := unfinishedHydrate()
	require.NoError(t, st.save(dir, catalog.EntityMovie))
	_, _, err = resolveHydrateSettings(hydrateOptions{dir: dir, entity: movies, limit: 10, apiRateLimit: 40})
	assert.ErrorIs(t, err, errUsage, "unfinished run needs --resume or --fresh")

	_, got, err = resolveHydrateSettings(hydrateOptions{dir: dir, entity: movies, apiRateLimit: 40, resume: true})
	require.NoError(t, err)
	assert.Equal(t, hydrateSettings{limit: 500, apiRateLimit: 5, minPopularity: 1, resuming: true}, got, "resume restores limit, rate, and popularity floor")

	_, got, err = resolveHydrateSettings(hydrateOptions{dir: dir, entity: movies, limit: 900, isLimitSet: true, apiRateLimit: 40, isRateLimitSet: true, minPopularity: 2, isMinPopularitySet: true, resume: true})
	require.NoError(t, err)
	assert.Equal(t, hydrateSettings{limit: 900, apiRateLimit: 40, minPopularity: 2, resuming: true}, got, "flags given again win")
}

func TestState_BeginHydrateRun(t *testing.T) {
	st := unfinishedHydrate()
	st.beginHydrateRun(hydrateSettings{limit: 500, apiRateLimit: 5, resuming: true})
	assert.Equal(t, 3, st.Hydrate.Runs, "resuming counts one more run on the same record")

	st.beginHydrateRun(hydrateSettings{limit: 10, apiRateLimit: 40})
	assert.Equal(t, 1, st.Hydrate.Runs, "a fresh run replaces the record")
	assert.Equal(t, 10, st.Hydrate.Limit)
	assert.True(t, st.hasUnfinishedHydrate())
	st.Hydrate.finish()
	assert.False(t, st.hasUnfinishedHydrate())
}

func TestOptionsValidate(t *testing.T) {
	movies := entityFlag{entity: catalog.EntityMovie}
	assert.ErrorIs(t, saveOptions{file: "x"}.validate(), errUsage, "entity required")
	assert.ErrorIs(t, saveOptions{entity: movies}.validate(), errUsage, "file required")
	assert.ErrorIs(t, saveOptions{entity: movies, file: "x", resume: true, fresh: true}.validate(), errUsage)
	assert.NoError(t, saveOptions{entity: movies, file: "x"}.validate())

	assert.ErrorIs(t, updateOptions{start: "2026-01-01"}.validate(), errUsage, "start without end")
	assert.ErrorIs(t, updateOptions{start: "2026-01-02", end: "2026-01-01"}.validate(), errUsage, "start after end")
	assert.ErrorIs(t, updateOptions{start: "01/01/2026", end: "2026-01-02"}.validate(), errUsage, "date format")
	assert.NoError(t, updateOptions{}.validate(), "no window means the watermark decides")
	assert.NoError(t, updateOptions{start: "2026-01-01", end: "2026-01-01"}.validate())

	assert.ErrorIs(t, hydrateOptions{}.validate(), errUsage, "entity required")
	assert.ErrorIs(t, hydrateOptions{entity: movies, resume: true, fresh: true}.validate(), errUsage)
	assert.ErrorIs(t, hydrateOptions{entity: movies}.validate(), errUsage, "limit required")
	assert.NoError(t, hydrateOptions{entity: movies, limit: 1}.validate())
	assert.NoError(t, hydrateOptions{entity: movies, resume: true}.validate(), "resume restores the limit from the checkpoint")
}

func TestEntityFlag(t *testing.T) {
	var f entityFlag
	assert.Empty(t, f.String())
	assert.False(t, f.isSet())
	require.NoError(t, f.Set("series"))
	assert.Equal(t, catalog.EntitySeries, f.entity)
	require.NoError(t, f.Set("people"))
	assert.Equal(t, "people", f.String())
	assert.Error(t, f.Set("seasons"), "seasons have no export")
	assert.Error(t, f.Set("movie"), "entities are spelled like their tables")
}

func TestExitCodes(t *testing.T) {
	assert.Equal(t, exitOK, exitCode(nil))
	assert.Equal(t, exitUsage, exitCode(usageErrorf("bad flag")))
	assert.Equal(t, exitInterrupted, exitCode(interruptedIfCancelled(context.Canceled)))
	assert.Equal(t, exitError, exitCode(errors.New("boom")))
	assert.Equal(t, exitError, exitCode(interruptedIfCancelled(errors.New("not a cancellation"))))
}

func TestRootCommand_UsageErrors(t *testing.T) {
	cases := map[string][]string{
		"unknown flag":       {"save", "--bogus"},
		"bad entity":         {"hydrate", "--entity", "seasons", "--limit", "1"},
		"missing entity":     {"save", "--file", "x"},
		"missing limit":      {"hydrate", "--entity", "movies"},
		"half a window":      {"update", "--start", "2026-01-01"},
		"resume and fresh":   {"hydrate", "--entity", "movies", "--limit", "1", "--resume", "--fresh"},
		"positional args":    {"status", "extra"},
		"unknown subcommand": {"nonsense"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			root := newRootCmd()
			root.SetArgs(args)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			err := root.Execute()
			require.Error(t, err)
			if name != "unknown subcommand" { // cobra reports unknown subcommands with an untyped error
				assert.ErrorIs(t, err, errUsage, "%v", err)
			}
		})
	}
}

func TestTerminal_NonTTYPrintsPlainLines(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "term")
	require.NoError(t, err)
	defer f.Close()

	term := newTerminal(f)
	assert.False(t, term.isTTY)
	term.setStatus("progress 1")
	term.setStatus("progress 2") // within the plain period: not printed
	_, err = term.Write([]byte("a log line\n"))
	require.NoError(t, err)
	term.finish()

	out, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, "progress 1\na log line\nprogress 2\n", string(out))
	assert.NotContains(t, string(out), "\033[K", "no escape codes off a TTY")
}

func TestProgressBarAndEta(t *testing.T) {
	assert.Equal(t, "[#####.....]", progressBar(50, 100, 10))
	assert.Equal(t, "[##########]", progressBar(200, 100, 10))
	assert.Equal(t, "", progressBar(1, 0, 10))
	assert.Equal(t, "--", eta(10, 0))
	assert.Equal(t, "5s", eta(10, 2))
}

func TestHydrateProgress_RecordRow(t *testing.T) {
	st := &state{}
	st.beginHydrateRun(hydrateSettings{limit: 4, apiRateLimit: 40})
	f, err := os.CreateTemp(t.TempDir(), "term")
	require.NoError(t, err)
	defer f.Close()
	p := &hydrateProgress{st: st, dir: t.TempDir(), entity: catalog.EntityMovie, term: newTerminal(f), limit: 4, attemptedBefore: 1, start: time.Now()}

	p.recordRow(catalog.RowResult{SourceID: 1, Outcome: catalog.OutcomeOK, PeopleFetched: 3})
	p.recordRow(catalog.RowResult{SourceID: 2, Outcome: catalog.OutcomeGone})
	p.recordRow(catalog.RowResult{SourceID: 3, Outcome: catalog.OutcomeFailed, Err: errors.New("boom")})

	assert.Equal(t, catalog.HydrateStats{Attempted: 3, OK: 1, Gone: 1, Failed: 1, PeopleFetched: 3}, p.run)
	assert.Equal(t, 1, st.Hydrate.OK)
	assert.Equal(t, []failure{{ID: 3, Count: 1, LastError: "boom"}}, st.Hydrate.Failures)
	assert.Contains(t, p.statusLine(), "4/4  ok 1  failed 1  gone 1  people 3")
}
