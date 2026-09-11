package main

import (
	"fmt"
	"io"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	checkpointEvery = 25
	barWidth        = 20
)

var numbers = message.NewPrinter(language.English)

type saveProgress struct {
	st     *state
	dir    string
	entity catalog.Entity
	term   *terminal
	bytes  *countingReader
	start  time.Time
}

type hydrateProgress struct {
	st              *state
	dir             string
	entity          catalog.Entity
	term            *terminal
	limit           int
	resuming        bool
	attemptedBefore int
	elapsedBefore   float64
	start           time.Time
	run             catalog.HydrateStats
}

type refreshProgress struct {
	entity catalog.Entity
	term   *terminal
	start  time.Time
	run    catalog.HydrateStats
}

// countingReader counts bytes so the load status line can show throughput.
type countingReader struct {
	r io.Reader
	n int64
}

// Read is the io.Reader side of countingReader.
func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (p *saveProgress) recordBatch(b catalog.BatchResult) {
	p.st.Save.LinesDone = b.LinesRead
	p.st.Save.Upserted += b.Upserted
	p.st.Save.Skipped += b.Skipped
	p.st.checkpoint(p.dir, p.entity)
	p.term.setStatus(p.statusLine())
	log.Debug().Int("lines", b.LinesRead).Int("upserted", b.Upserted).Int("skipped", b.Skipped).Msg("batch committed")
}

func (p *saveProgress) statusLine() string {
	elapsed := time.Since(p.start)
	mbps := float64(p.bytes.n) / 1e6 / max(elapsed.Seconds(), 0.001)
	return fmt.Sprintf("save %s  %s lines  %s upserted  %s skipped  %.1f MB/s  elapsed %s",
		p.entity, numbers.Sprint(p.st.Save.LinesDone), numbers.Sprint(p.st.Save.Upserted), numbers.Sprint(p.st.Save.Skipped),
		mbps, elapsed.Round(time.Second))
}

func (p *hydrateProgress) recordRow(r catalog.RowResult) {
	h := p.st.Hydrate
	p.run.Record(r)
	h.PeopleFetched += r.PeopleFetched
	switch r.Outcome {
	case catalog.OutcomeOK:
		h.OK++
	case catalog.OutcomeGone:
		h.Gone++
	case catalog.OutcomeFailed:
		h.Failed++
		h.recordFailure(r.SourceID, r.Err)
	}
	logRow(p.entity, r)
	p.term.setStatus(p.statusLine())
	if p.run.Attempted%checkpointEvery == 0 {
		p.checkpoint()
	}
}

func logRow(entity catalog.Entity, r catalog.RowResult) {
	event := log.Debug()
	msg := "row hydrated"
	switch {
	case r.Outcome == catalog.OutcomeGone:
		event, msg = log.Info(), "gone: TMDB returned 404, row deleted"
	case r.Outcome == catalog.OutcomeFailed:
		event, msg = log.Warn().Err(r.Err), "row failed"
	case r.Err != nil:
		event, msg = log.Warn().Err(r.Err), "row stored, post-commit work incomplete"
	}
	event.Str("entity", entity.String()).Int("source_id", r.SourceID).Int("people", r.PeopleFetched).Msg(msg)
}

func (p *hydrateProgress) statusLine() string {
	h := p.st.Hydrate
	elapsed := max(time.Since(p.start).Seconds(), 0.001)
	done := p.attemptedBefore + p.run.Attempted
	rowsPerSec := float64(p.run.Attempted) / elapsed
	reqPerSec := float64(p.run.Attempted+p.run.PeopleFetched) / elapsed
	resumed := ""
	if p.resuming {
		resumed = " (resumed)"
	}
	return fmt.Sprintf("hydrate %s  %s %s/%s  ok %s  failed %s  gone %s  people %s  %.1f req/s  eta %s  run %d%s",
		p.entity, progressBar(done, p.limit, barWidth), numbers.Sprint(done), numbers.Sprint(p.limit),
		numbers.Sprint(h.OK), numbers.Sprint(h.Failed), numbers.Sprint(h.Gone), numbers.Sprint(h.PeopleFetched),
		reqPerSec, eta(p.limit-done, rowsPerSec), h.Runs, resumed)
}

func (p *refreshProgress) recordRow(r catalog.RowResult) {
	p.run.Record(r)
	logRow(p.entity, r)
	p.term.setStatus(p.statusLine())
}

func (p *refreshProgress) statusLine() string {
	elapsed := time.Since(p.start)
	reqPerSec := float64(p.run.Attempted+p.run.PeopleFetched) / max(elapsed.Seconds(), 0.001)
	return fmt.Sprintf("update %s  refreshed %s  failed %s  gone %s  people %s  %.1f req/s  elapsed %s",
		p.entity, numbers.Sprint(p.run.OK), numbers.Sprint(p.run.Failed), numbers.Sprint(p.run.Gone),
		numbers.Sprint(p.run.PeopleFetched), reqPerSec, elapsed.Round(time.Second))
}

func (p *hydrateProgress) checkpoint() {
	p.st.Hydrate.ElapsedSeconds = p.elapsedBefore + time.Since(p.start).Seconds()
	p.st.checkpoint(p.dir, p.entity)
}

func progressBar(done, total, width int) string {
	if total <= 0 {
		return ""
	}
	filled := min(width, done*width/total)
	bar := make([]byte, width)
	for i := range bar {
		bar[i] = '.'
		if i < filled {
			bar[i] = '#'
		}
	}
	return "[" + string(bar) + "]"
}

func eta(remaining int, perSecond float64) string {
	if perSecond <= 0 || remaining <= 0 {
		return "--"
	}
	return (time.Duration(float64(remaining)/perSecond) * time.Second).Round(time.Second).String()
}
