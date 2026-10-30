package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// changesWindowDays is the widest date range a changes endpoint accepts in one call.
	changesWindowDays = 14
	// maxChangesPages is the deepest page a changes endpoint serves; a window with more pages has to be split.
	maxChangesPages = 500
)

type changesResponse struct {
	Results []struct {
		ID int `json:"id"`
	} `json:"results"`
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

// GetChanges returns the ids of a media type that changed between startDate and endDate (YYYY-MM-DD, inclusive).
// Ranges wider than the feed's window are walked in consecutive windows, a window with more changes
// than the feed pages is halved until it fits, and ids are deduplicated.
func (c *Client) GetChanges(ctx context.Context, mediaType MediaType, startDate, endDate string) ([]int, error) {
	start, err := time.Parse(time.DateOnly, startDate)
	if err != nil {
		return nil, fmt.Errorf("changes start date: %w", err)
	}
	end, err := time.Parse(time.DateOnly, endDate)
	if err != nil {
		return nil, fmt.Errorf("changes end date: %w", err)
	}

	var ids []int
	seen := make(map[int]struct{})
	for from := start; !from.After(end); from = from.AddDate(0, 0, changesWindowDays) {
		to := from.AddDate(0, 0, changesWindowDays-1)
		if to.After(end) {
			to = end
		}
		windowIDs, err := c.changesBetween(ctx, mediaType, from, to)
		if err != nil {
			return nil, err
		}
		for _, id := range windowIDs {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

func (c *Client) changesBetween(ctx context.Context, mediaType MediaType, from, to time.Time) ([]int, error) {
	fromDate, toDate := from.Format(time.DateOnly), to.Format(time.DateOnly)
	var ids []int
	for page := 1; ; page++ {
		body, err := c.get(ctx, fmt.Sprintf("/%s/changes?start_date=%s&end_date=%s&page=%d", mediaType, fromDate, toDate, page))
		if err != nil {
			return nil, fmt.Errorf("%s changes %s to %s page %d: %w", mediaType, fromDate, toDate, page, err)
		}
		var res changesResponse
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, fmt.Errorf("parse %s changes: %w", mediaType, err)
		}
		if page == 1 && res.TotalPages > maxChangesPages {
			if from.Equal(to) {
				return nil, fmt.Errorf("%s changes on %s: %d pages, the feed serves %d", mediaType, fromDate, res.TotalPages, maxChangesPages)
			}
			mid := from.AddDate(0, 0, int(to.Sub(from).Hours()/24)/2)
			first, err := c.changesBetween(ctx, mediaType, from, mid)
			if err != nil {
				return nil, err
			}
			second, err := c.changesBetween(ctx, mediaType, mid.AddDate(0, 0, 1), to)
			if err != nil {
				return nil, err
			}
			return append(first, second...), nil
		}
		for _, r := range res.Results {
			ids = append(ids, r.ID)
		}
		if page >= res.TotalPages {
			log.Debug().Str("media_type", string(mediaType)).Str("from", fromDate).Str("to", toDate).Int("pages", page).
				Int("ids", len(ids)).Msg("changes window read")
			return ids, nil
		}
	}
}
