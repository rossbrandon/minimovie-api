package catalog

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/rossbrandon/minimovie-api/internal/store"
)

const (
	seedBatchSize     = 10000
	seedMaxLineBytes  = 1 << 20
	seedScannerBuffer = 64 << 10
)

type exportLine struct {
	ID            int     `json:"id"`
	OriginalTitle string  `json:"original_title"`
	OriginalName  string  `json:"original_name"`
	Name          string  `json:"name"`
	Popularity    float64 `json:"popularity"`
	Adult         bool    `json:"adult"`
}

type BatchResult struct {
	LinesRead int
	Upserted  int
	Skipped   int
}

// SeedExports streams a gzipped id export into skeleton rows.
func (s *Service) SeedExports(
	ctx context.Context,
	entity Entity,
	r io.Reader,
	skipLines int,
	onBatch func(BatchResult),
) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("catalog: open export: %w", err)
	}
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, seedScannerBuffer), seedMaxLineBytes)

	var (
		lines   int
		batch   []exportLine
		skipped int
	)
	flush := func() error {
		if len(batch) == 0 && skipped == 0 {
			return nil
		}
		if err := s.upsertSkeletons(ctx, entity, batch); err != nil {
			return err
		}
		if onBatch != nil {
			onBatch(BatchResult{LinesRead: lines, Upserted: len(batch), Skipped: skipped})
		}
		batch, skipped = batch[:0], 0
		return nil
	}

	for scanner.Scan() {
		lines++
		if lines <= skipLines {
			continue
		}
		var line exportLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return fmt.Errorf("catalog: export line %d: %w", lines, err)
		}
		if line.Adult {
			skipped++
		} else {
			batch = append(batch, line)
		}
		if len(batch) >= seedBatchSize {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("catalog: read export: %w", err)
	}
	return flush()
}

func (l exportLine) title() string {
	switch {
	case l.OriginalTitle != "":
		return l.OriginalTitle
	case l.OriginalName != "":
		return l.OriginalName
	}
	return l.Name
}

func (s *Service) upsertSkeletons(ctx context.Context, entity Entity, lines []exportLine) error {
	switch entity {
	case EntityMovie, EntitySeries:
		rows := make([]store.Skeleton, len(lines))
		for i, l := range lines {
			rows[i] = store.Skeleton{SourceID: l.ID, Title: l.title(), Popularity: l.Popularity}
		}
		if entity == EntityMovie {
			return s.movies.UpsertSkeleton(ctx, s.pool, rows)
		}
		return s.series.UpsertSkeleton(ctx, s.pool, rows)
	case EntityPerson:
		rows := make([]store.PersonSkeleton, len(lines))
		for i, l := range lines {
			rows[i] = store.PersonSkeleton{SourceID: l.ID, Name: l.title(), Popularity: l.Popularity}
		}
		return s.people.UpsertSkeleton(ctx, s.pool, rows)
	}
	return fmt.Errorf("catalog: cannot seed entity %s", entity)
}
