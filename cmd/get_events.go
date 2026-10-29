package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// EventListItem is the table row for events.
type EventListItem struct {
	EventID        string `table:"EVENT-ID"`
	EventType      string `table:"TYPE"`
	Title          string `table:"TITLE"`
	StartTime      string `table:"START-TIME"`
	EndTime        string `table:"END-TIME,wide"`
	AffectedEntity string `table:"ENTITY,wide"`
}

// EventsResponse models /api/v2/events.
type EventsResponse struct {
	Events     []EventEntry `json:"events"`
	TotalCount int          `json:"totalCount"`
}

type EventEntry struct {
	EventID        string `json:"eventId"`
	EventType      string `json:"eventType"`
	Title          string `json:"title"`
	StartTime      int64  `json:"startTime"`
	EndTime        int64  `json:"endTime"`
	AffectedEntity *struct {
		EntityID string `json:"entityId"`
		Name     string `json:"name"`
		Type     string `json:"type"`
	} `json:"affectedEntity"`
}

func newGetEventsCmd(a *App) *cobra.Command {
	var opts struct {
		from      string
		to        string
		eventType string
		entity    string
		limit     int
	}
	var watch watchOptions
	cmd := &cobra.Command{
		Use:     "events",
		Aliases: []string{"ev", "event"},
		Short:   "List events from the Dynatrace Managed environment",
		Long: `List events within a time range.

Examples:
  dtmgd get events --from now-1h --to now
  dtmgd get events --from now-6h --type CUSTOM_DEPLOYMENT
  dtmgd get events --env ALL_ENVIRONMENTS --from now-1h`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.from == "" {
				return &UsageError{Err: fmt.Errorf("--from is required (e.g. now-1h)")}
			}
			return a.watchOrRun(watch, func() error {
				to := opts.to
				if to == "" {
					to = "now"
				}

				params := map[string]string{
					"from": opts.from,
					"to":   to,
				}
				if opts.eventType != "" {
					params["eventType"] = opts.eventType
				}
				if opts.entity != "" {
					params["entitySelector"] = opts.entity
				}

				return a.runFetch(fetchSpec{
					Verb:     "get",
					Resource: "events",
					Fetch:    pagedFetch("/events", params, "events", 100, a.Opts.MaxPages),
					Rows:     eventRows,
					Limit:    opts.limit,
					List:     true,
					Empty:    "No events found for the given filters.",
				})
			})
		},
	}
	addWatchFlags(cmd, &watch)
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (required), e.g. now-1h")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().StringVar(&opts.eventType, "type", "", "filter by event type (e.g. CUSTOM_DEPLOYMENT)")
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entity selector to filter events")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of events")
	return withScopes(cmd, scopeEventsRead)
}

// eventRows turns an /events body into table rows.
func eventRows(body interface{}) (interface{}, int, error) {
	var resp EventsResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]EventListItem, 0, len(resp.Events))
	for _, e := range resp.Events {
		entity := ""
		if e.AffectedEntity != nil {
			entity = fmt.Sprintf("%s (%s)", e.AffectedEntity.Name, e.AffectedEntity.Type)
		}
		items = append(items, EventListItem{
			EventID:        e.EventID,
			EventType:      e.EventType,
			Title:          e.Title,
			StartTime:      msToTime(e.StartTime),
			EndTime:        msToTimeOrEmpty(e.EndTime),
			AffectedEntity: entity,
		})
	}
	return items, len(items), nil
}
