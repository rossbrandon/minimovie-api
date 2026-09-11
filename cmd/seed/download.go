package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const exportsBaseURL = "https://files.tmdb.org/p/exports/"
const downloadLongDesc = `
Download the daily id export files for each entity (movies, series, people) from TMDB.
Use the --entity flag to specify a different entity (movie, series, person). Defaults to all three.
Use the --date flag to specify a different date (MM_DD_YYYY). Defaults to yesterday's date.
`

// exportFiles maps an entity to the name of its daily id export file.
var exportFiles = map[catalog.Entity]string{
	catalog.EntityMovie:  "movie_ids",
	catalog.EntitySeries: "tv_series_ids",
	catalog.EntityPerson: "person_ids",
}

type downloadOptions struct {
	dir    string
	date   string
	entity entityFlag // unset means all three
}

type downloader struct {
	client *http.Client
	term   *terminal
}

// downloadProgress is an io.Writer that only updates the status line.
type downloadProgress struct {
	term    *terminal
	name    string
	total   int64
	written int64
}

// Write is the io.Writer side of downloadProgress.
func (p *downloadProgress) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if p.total > 0 {
		p.term.setStatus(fmt.Sprintf("%s  %s  %.1f / %.1f MB", p.name, progressBar(int(p.written), int(p.total), barWidth),
			float64(p.written)/1e6, float64(p.total)/1e6))
	} else {
		p.term.setStatus(fmt.Sprintf("%s  %.1f MB", p.name, float64(p.written)/1e6))
	}
	return len(b), nil
}

func newDownloadCmd(g *globalOptions) *cobra.Command {
	var o downloadOptions
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download the daily id export files from TMDB",
		Long:  downloadLongDesc,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.dir = g.dir
			return runDownload(cmd.Context(), o)
		},
	}
	cmd.Flags().StringVar(&o.date, "date", yesterday(), "export date to download (MM_DD_YYYY). Defaults to yesterday's date.")
	cmd.Flags().Var(&o.entity, "entity", "download only this entity's file (movies, series, or people)")
	return cmd
}

func runDownload(ctx context.Context, o downloadOptions) error {
	if err := os.MkdirAll(o.dir, 0o755); err != nil {
		return err
	}
	term := newTerminal(os.Stderr)
	defer term.finish()
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: term, TimeFormat: "15:04:05"}).With().Timestamp().Logger()

	entities := seedEntities
	if o.entity.isSet() {
		entities = []catalog.Entity{o.entity.entity}
	}
	d := downloader{client: http.DefaultClient, term: term}
	for _, entity := range entities {
		file := fmt.Sprintf("%s_%s.json.gz", exportFiles[entity], o.date)
		url := exportsBaseURL + file
		dest, err := filepath.Abs(filepath.Join(o.dir, file))
		if err != nil {
			return err
		}
		log.Info().Str("entity", entity.String()).Str("url", url).Msg("downloading export")
		size, err := d.download(ctx, url, dest)
		if err != nil {
			return fmt.Errorf("%s: %w", file, interruptedIfCancelled(err))
		}
		log.Info().Str("entity", entity.String()).Str("path", dest).Str("size", fmt.Sprintf("%.1f MB", float64(size)/1e6)).Msg("saved export")
	}
	return nil
}

func yesterday() string {
	return time.Now().UTC().AddDate(0, 0, -1).Format("01_02_2006")
}

// download fetches url into dest through a .part file.
func (d downloader) download(ctx context.Context, url, dest string) (int64, error) {
	res, err := d.get(ctx, url)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	tmp := dest + ".part"
	size, err := d.save(res, tmp)
	if err != nil {
		return 0, err
	}
	return size, os.Rename(tmp, dest)
}

func (d downloader) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf(
			"could not download export file at %s: http status: %d. Requested date may not be available",
			url,
			res.StatusCode,
		)
	}
	return res, nil
}

func (d downloader) save(res *http.Response, path string) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	progress := &downloadProgress{term: d.term, name: filepath.Base(path), total: res.ContentLength}
	size, err := io.Copy(io.MultiWriter(f, progress), res.Body)
	if err != nil {
		_ = f.Close()
		return 0, err
	}
	return size, f.Close()
}
