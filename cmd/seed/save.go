package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type saveOptions struct {
	dir    string
	entity entityFlag
	file   string
	resume bool
	fresh  bool
}

func (o saveOptions) validate() error {
	switch {
	case !o.entity.isSet():
		return usageErrorf("--entity is required (movies, series, or people)")
	case o.file == "":
		return usageErrorf("--file is required")
	case o.resume && o.fresh:
		return usageErrorf("--resume and --fresh are mutually exclusive")
	}
	return nil
}

func newSaveCmd(g *globalOptions) *cobra.Command {
	var o saveOptions
	cmd := &cobra.Command{
		Use:   "save",
		Short: "Tier 1: save an id export file as skeleton rows, zero API calls",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.dir = g.dir
			if err := o.validate(); err != nil {
				return err
			}
			return runSave(cmd.Context(), o)
		},
	}
	cmd.Flags().Var(&o.entity, "entity", "movies, series, or people")
	cmd.Flags().StringVar(&o.file, "file", "", "path to a TMDB id export (.json.gz)")
	cmd.Flags().BoolVar(&o.resume, "resume", false, "continue an unfinished run from its last checkpoint")
	cmd.Flags().BoolVar(&o.fresh, "fresh", false, "discard an unfinished run's checkpoint and start over")
	return cmd
}

// runSave streams one export file into skeleton rows.
func runSave(ctx context.Context, o saveOptions) error {
	st, skipLines, err := resolveSaveStart(o)
	if err != nil {
		return err
	}

	s, err := openSession(ctx, sessionOptions{dir: o.dir, entity: o.entity.entity})
	if err != nil {
		return err
	}
	defer s.close()

	progress, runErr := streamExport(ctx, s, st, o, skipLines)
	return finishSave(progress, runErr)
}

// resolveSaveStart applies the rerun guard and returns the state plus the first line to read: 0 for
// a new run, or past the checkpoint of the same file with --resume.
func resolveSaveStart(o saveOptions) (*state, int, error) {
	info, err := os.Stat(o.file)
	if err != nil {
		return nil, 0, err
	}
	st, err := readState(o.dir, o.entity.entity)
	if err != nil {
		return nil, 0, err
	}
	if err := st.checkUnfinishedRun("save", o.resume, o.fresh); err != nil {
		return nil, 0, err
	}

	fileName := filepath.Base(o.file)
	if o.resume && st.hasUnfinishedSave() {
		if st.Save.File != fileName || st.Save.Size != info.Size() {
			return nil, 0, usageErrorf("--resume needs the same file as the unfinished run (%s, %d bytes)", st.Save.File, st.Save.Size)
		}
		return st, st.Save.LinesDone, nil
	}
	st.beginSave(fileName, info.Size())
	return st, 0, nil
}

// streamExport reads the file into skeleton rows through catalog.SeedExports
func streamExport(ctx context.Context, s *session, st *state, o saveOptions, skipLines int) (*saveProgress, error) {
	entity := o.entity.entity
	f, err := os.Open(o.file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := &countingReader{r: f}
	progress := &saveProgress{st: st, dir: o.dir, entity: entity, term: s.term, bytes: reader, start: time.Now()}
	log.Info().Str("entity", entity.String()).Str("file", st.Save.File).Int("skip_lines", skipLines).Msg("save started")

	err = s.catalog.SeedExports(ctx, entity, reader, skipLines, progress.recordBatch)
	return progress, err
}

// finishSave closes the record on success, checkpoints progress, and logs the outcome.
func finishSave(p *saveProgress, runErr error) error {
	if p == nil {
		return runErr
	}
	if runErr == nil {
		p.st.Save.finish()
	}
	p.st.checkpoint(p.dir, p.entity)
	log.Info().Str("entity", p.entity.String()).Int("lines", p.st.Save.LinesDone).Int("upserted", p.st.Save.Upserted).
		Int("skipped", p.st.Save.Skipped).Str("elapsed", time.Since(p.start).Round(time.Second).String()).
		Bool("finished", !p.st.Save.isUnfinished()).Msg("save ended")
	return interruptedIfCancelled(runErr)
}
