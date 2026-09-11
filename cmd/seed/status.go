package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newStatusCmd(g *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check the status of the seed process",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), g.dir, cmd.OutOrStdout())
		},
	}
}

func runStatus(ctx context.Context, dir string, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	svc, pool, err := connectCatalog(ctx, cfg, 0)
	if err != nil {
		return err
	}
	defer pool.Close()

	printTableCounts(ctx, svc, out)
	for _, entity := range seedEntities {
		st, err := readState(dir, entity)
		if err != nil {
			return err
		}
		printLastRuns(out, entity, st)
	}
	return nil
}

// printTableCounts writes the per-table counts.
func printTableCounts(ctx context.Context, svc *catalog.Service, out io.Writer) {
	stats, err := svc.Stats(ctx)
	if err != nil {
		log.Error().Err(err).Msg("stats failed")
		return
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "table\trows\thydrated\tstale\texpiring\tunhydrated>=5\toldest(days)")
	for _, s := range stats {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n", s.Table, numbers.Sprint(s.Rows), numbers.Sprint(s.Hydrated),
			numbers.Sprint(s.Stale), numbers.Sprint(s.Expiring), numbers.Sprint(s.UnhydratedPopular), s.OldestDays)
	}
	_ = w.Flush()
}

// printLastRuns writes one entity's last save and hydrate records with the failed ids.
func printLastRuns(out io.Writer, entity catalog.Entity, st *state) {
	if st.Save == nil && st.Hydrate == nil {
		return
	}
	fmt.Fprintf(out, "\n%s\n", entity)
	if l := st.Save; l != nil {
		fmt.Fprintf(out, "  save     %s  lines %s  upserted %s  skipped %s  %s\n",
			l.File, numbers.Sprint(l.LinesDone), numbers.Sprint(l.Upserted), numbers.Sprint(l.Skipped), finishedLabel(l.runRecord))
	}
	if h := st.Hydrate; h != nil {
		fmt.Fprintf(out, "  hydrate  limit %s  runs %d  ok %s  failed %s  gone %s  people %s  elapsed %s  %s\n",
			numbers.Sprint(h.Limit), h.Runs, numbers.Sprint(h.OK), numbers.Sprint(h.Failed), numbers.Sprint(h.Gone),
			numbers.Sprint(h.PeopleFetched), (time.Duration(h.ElapsedSeconds) * time.Second).Round(time.Second), finishedLabel(h.runRecord))
		printFailures(out, h.Failures)
	}
}

func printFailures(out io.Writer, failures []failure) {
	const shown = 5
	for i, f := range failures {
		if i == shown {
			fmt.Fprintf(out, "           ... %d more in the state file\n", len(failures)-shown)
			return
		}
		fmt.Fprintf(out, "           source id %d failed %dx: %s\n", f.ID, f.Count, f.LastError)
	}
}

func finishedLabel(r runRecord) string {
	if r.isUnfinished() {
		return "UNFINISHED since " + r.StartedAt.Format(time.RFC3339) + " (--resume to continue)"
	}
	return "finished " + r.FinishedAt.Format(time.RFC3339)
}
