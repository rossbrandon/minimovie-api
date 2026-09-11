package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const (
	defaultDir = "local-development/exports"

	exitOK          = 0
	exitError       = 1
	exitUsage       = 2
	exitInterrupted = 130
)

const rootLongDesc = `seed loads TMDB's daily id exports into the catalog and hydrates the most popular rows.

Environment variable requirements: DATABASE_URL, TMDB_ACCESS_TOKEN.
Each seed process logs to DIR/seed-<entity>.log.
Checkpoints DIR/state/<entity>.json are created after every batch, so the process can be
resumed with the --resume flag.
Exit codes: 0 done, 1 failed, 2 usage or guard, 130 interrupted.`

var (
	errUsage       = errors.New("usage error")
	errInterrupted = errors.New("interrupted")
	seedEntities   = []catalog.Entity{catalog.EntityPerson, catalog.EntityMovie, catalog.EntitySeries}
)

type globalOptions struct {
	dir string
}

type entityFlag struct {
	entity catalog.Entity
}

func (f *entityFlag) String() string {
	if f.entity == catalog.EntityUnknown {
		return ""
	}
	return f.entity.String()
}

func (f *entityFlag) Set(s string) error {
	k, err := catalog.ParseEntity(s)
	if err != nil || !slices.Contains(seedEntities, k) {
		return errors.New("must be movies, series, or people")
	}
	f.entity = k
	return nil
}

func (f *entityFlag) Type() string {
	return "entity"
}

func newRootCmd() *cobra.Command {
	g := &globalOptions{}
	root := &cobra.Command{
		Use:           "seed",
		Short:         "Load source id exports into the catalog and hydrate the most popular rows",
		Long:          rootLongDesc,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).With().Timestamp().Logger()
		},
	}
	root.PersistentFlags().StringVar(&g.dir, "dir", defaultDir, "directory for export files, logs, and state")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %v", errUsage, err)
	})
	root.AddCommand(newDownloadCmd(g), newSaveCmd(g), newHydrateCmd(g), newUpdateCmd(g), newStatusCmd(g))
	return root
}

func exitCode(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, errUsage):
		return exitUsage
	case errors.Is(err, errInterrupted):
		return exitInterrupted
	default:
		return exitError
	}
}

func interruptedIfCancelled(err error) error {
	if errors.Is(err, context.Canceled) {
		return errInterrupted
	}
	return err
}

func usageErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, args...))
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	return nil
}

// signalContext cancels on the first Ctrl-C and then restores default handling.
// The first signal lets the current batch finish and checkpoint its progress.
// The second signal kills the process outright.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx, stop
}

func (f *entityFlag) isSet() bool {
	return f.entity != catalog.EntityUnknown
}
