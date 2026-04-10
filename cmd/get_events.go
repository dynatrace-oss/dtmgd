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
	EventID   string `json:"eventId"`
	EventType string `json:"eventType"`
	Title     string `json:"title"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	AffectedEntity *struct {
		EntityID string `json:"entityId"`
		Name     string `json:"name"`
		Type     string `json:"type"`
	} `json:"affectedEntity"`
}

var (
	evFrom   string
	evTo     string
	evType   string
	evEntity string
	evLimit  int
)

var getEventsCmd = &cobra.Command{
	Use:     "events",
	Aliases: []string{"ev", "event"},
	Short:   "List events from the Dynatrace Managed environment",
	Long: `List events within a time range.

Examples:
  dtmgd get events --from now-1h --to now
  dtmgd get events --from now-6h --type CUSTOM_DEPLOYMENT
  dtmgd get events --env ALL_ENVIRONMENTS --from now-1h`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return watchOrRun(func() error {
			if evFrom == "" {
				return fmt.Errorf("--from is required (e.g. now-1h)")
			}
			if evTo == "" {
				evTo = "now"
			}

			cfg, err := LoadConfig()
			if err != nil {
				return err
			}

			params := map[string]string{
				"from": evFrom,
				"to":   evTo,
			}
			if evType != "" {
				params["eventType"] = evType
			}
			if evEntity != "" {
				params["entitySelector"] = evEntity
			}
			pageSize := 100
			if evLimit > 0 {
				pageSize = evLimit
			}
			params["pageSize"] = fmt.Sprintf("%d", pageSize)

			if isMultiEnv() {
				data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
					return c.GetV2Paged("/events", params, effectiveMaxPages(evLimit > 0))
				})
				if err != nil {
					return err
				}
				return NewPrinterForResource("events").Print(data)
			}

			c, err := NewClientFromConfig(cfg)
			if err != nil {
				return err
			}

			raw, err := c.GetV2Paged("/events", params, effectiveMaxPages(evLimit > 0))
			if err != nil {
				return err
			}

			var resp EventsResponse
			if err := client.DecodePaged(raw, &resp); err != nil {
				return err
			}

			if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
				return NewPrinterForResource("events").Print(resp)
			}

			events := resp.Events
			if evLimit > 0 && len(events) > evLimit {
				events = events[:evLimit]
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
			return NewPrinter().PrintList(items)
		})
	},
}

func init() {
	getCmd.AddCommand(getEventsCmd)

	getEventsCmd.Flags().StringVar(&evFrom, "from", "", "start time (required), e.g. now-1h")
	getEventsCmd.Flags().StringVar(&evTo, "to", "", "end time (default: now)")
	getEventsCmd.Flags().StringVar(&evType, "type", "", "filter by event type (e.g. CUSTOM_DEPLOYMENT)")
	getEventsCmd.Flags().StringVar(&evEntity, "entity", "", "entity selector to filter events")
	getEventsCmd.Flags().IntVar(&evLimit, "limit", 0, "maximum number of events")
}
