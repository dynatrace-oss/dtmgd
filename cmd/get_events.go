package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
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
			return a.watchOrRun(func() error {
				if opts.from == "" {
					return fmt.Errorf("--from is required (e.g. now-1h)")
				}
				to := opts.to
				if to == "" {
					to = "now"
				}

				cfg, err := a.loadConfig()
				if err != nil {
					return err
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
				pageSize := 100
				if opts.limit > 0 {
					pageSize = opts.limit
				}
				params["pageSize"] = fmt.Sprintf("%d", pageSize)

				if a.isMultiEnv() {
					data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
						return c.GetV2Paged("/events", params, client.Paging{MaxPages: a.maxPagesFor(opts.limit > 0)})
					})
					if err != nil {
						return err
					}
					return a.printer("events").Print(data)
				}

				c, err := a.newClient(cfg)
				if err != nil {
					return err
				}

				raw, err := c.GetV2Paged("/events", params, client.Paging{MaxPages: a.maxPagesFor(opts.limit > 0)})
				if err != nil {
					return err
				}

				var resp EventsResponse
				if err := client.DecodePaged(raw, &resp); err != nil {
					return err
				}

				if a.Opts.Output == "json" || a.Opts.Output == "yaml" || a.agentMode() {
					return a.printer("events").Print(resp)
				}

				events := resp.Events
				if opts.limit > 0 && len(events) > opts.limit {
					events = events[:opts.limit]
				}

				if len(events) == 0 {
					output.PrintInfo("No events found for the given filters.")
					return nil
				}

				var items []EventListItem
				for _, e := range events {
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

				if resp.TotalCount > len(events) {
					output.PrintInfo("Showing %d of %d events.", len(events), resp.TotalCount)
				}
				return a.printer("").PrintList(items)
			})
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (required), e.g. now-1h")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().StringVar(&opts.eventType, "type", "", "filter by event type (e.g. CUSTOM_DEPLOYMENT)")
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entity selector to filter events")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of events")
	return withScopes(cmd, "events.read")
}
