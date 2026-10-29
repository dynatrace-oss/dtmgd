package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// EntityTypeListItem is the table row for entity types.
type EntityTypeListItem struct {
	Type        string `table:"TYPE"`
	DisplayName string `table:"DISPLAY-NAME"`
	Properties  int    `table:"PROPERTIES,wide"`
}

// EntityListItem is the table row for discovered entities.
type EntityListItem struct {
	EntityID    string `table:"ENTITY-ID"`
	Type        string `table:"TYPE"`
	DisplayName string `table:"DISPLAY-NAME"`
	FirstSeen   string `table:"FIRST-SEEN,wide"`
	LastSeen    string `table:"LAST-SEEN,wide"`
}

// EntityTypesResponse models /api/v2/entityTypes.
type EntityTypesResponse struct {
	Types      []EntityTypeEntry `json:"types"`
	TotalCount int               `json:"totalCount"`
}

type EntityTypeEntry struct {
	Type        string        `json:"type"`
	DisplayName string        `json:"displayName"`
	Properties  []interface{} `json:"properties"`
}

// EntitiesResponse models /api/v2/entities.
type EntitiesResponse struct {
	Entities   []EntityEntry `json:"entities"`
	TotalCount int           `json:"totalCount"`
}

type EntityEntry struct {
	EntityID     string `json:"entityId"`
	Type         string `json:"type"`
	DisplayName  string `json:"displayName"`
	FirstSeenTms int64  `json:"firstSeenTms"`
	LastSeenTms  int64  `json:"lastSeenTms"`
}

func newGetEntityTypesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "entity-types",
		Aliases: []string{"et", "entity-type"},
		Short:   "List all available entity types",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					raw, err := c.GetV2Paged("/entityTypes", map[string]string{"pageSize": "500"}, client.Paging{MaxPages: a.Opts.MaxPages})
					if err != nil {
						return nil, err
					}
					return raw, nil
				})
				if err != nil {
					return err
				}
				return a.printer("entityTypes").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			raw, err := c.GetV2Paged("/entityTypes", map[string]string{"pageSize": "500"}, client.Paging{MaxPages: a.Opts.MaxPages})
			if err != nil {
				return err
			}

			var resp EntityTypesResponse
			if err := client.DecodePaged(raw, &resp); err != nil {
				return err
			}

			if a.Opts.Output == "json" || a.Opts.Output == "yaml" || a.agentMode() {
				return a.printer("entityTypes").Print(resp)
			}

			var items []EntityTypeListItem
			for _, t := range resp.Types {
				items = append(items, EntityTypeListItem{
					Type:        t.Type,
					DisplayName: t.DisplayName,
					Properties:  len(t.Properties),
				})
			}
			if len(items) == 0 {
				output.PrintInfo("No entity types found.")
				return nil
			}
			return a.printer("").PrintList(items)
		},
	}
	return withScopes(cmd, "entities.read")
}

func newGetEntitiesCmd(a *App) *cobra.Command {
	var opts struct {
		selector string
		from     string
		to       string
		limit    int
		sort     string
		mz       string
	}
	cmd := &cobra.Command{
		Use:     "entities",
		Aliases: []string{"ent", "entity"},
		Short:   "Discover entities using an entity selector",
		Long: `Discover monitored entities using the Dynatrace entity selector syntax.
--selector is required and must specify exactly ONE entity type.

Examples:
  dtmgd get entities --selector 'type(SERVICE)'
  dtmgd get entities --selector 'type(HOST),healthState("HEALTHY")'
  dtmgd get entities --env ALL_ENVIRONMENTS --selector 'type(SERVICE)'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.selector == "" {
				return fmt.Errorf("--selector is required (e.g. 'type(SERVICE)')")
			}

			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			params := map[string]string{
				"entitySelector": opts.selector,
			}
			pageSize := 100
			if opts.limit > 0 {
				pageSize = opts.limit
			}
			params["pageSize"] = fmt.Sprintf("%d", pageSize)
			if opts.from != "" {
				params["from"] = opts.from
			}
			if opts.to != "" {
				params["to"] = opts.to
			}
			if opts.sort != "" {
				params["sort"] = opts.sort
			}
			if opts.mz != "" {
				params["mzSelector"] = opts.mz
			}

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					raw, err := c.GetV2Paged("/entities", params, client.Paging{MaxPages: a.maxPagesFor(opts.limit > 0)})
					if err != nil {
						return nil, err
					}
					return raw, nil
				})
				if err != nil {
					return err
				}
				return a.printer("entities").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			raw, err := c.GetV2Paged("/entities", params, client.Paging{MaxPages: a.maxPagesFor(opts.limit > 0)})
			if err != nil {
				return err
			}

			var resp EntitiesResponse
			if err := client.DecodePaged(raw, &resp); err != nil {
				return err
			}

			if a.Opts.Output == "json" || a.Opts.Output == "yaml" || a.agentMode() {
				return a.printer("entities").Print(resp)
			}

			var items []EntityListItem
			for _, e := range resp.Entities {
				fs := ""
				ls := ""
				if e.FirstSeenTms > 0 {
					fs = msToTime(e.FirstSeenTms)
				}
				if e.LastSeenTms > 0 {
					ls = msToTime(e.LastSeenTms)
				}
				items = append(items, EntityListItem{
					EntityID:    e.EntityID,
					Type:        e.Type,
					DisplayName: e.DisplayName,
					FirstSeen:   fs,
					LastSeen:    ls,
				})
			}

			if len(items) == 0 {
				output.PrintInfo("No entities found for selector: %s", opts.selector)
				return nil
			}
			if resp.TotalCount > len(items) {
				output.PrintInfo("Showing %d of %d entities. Use --limit or a more specific selector.", len(items), resp.TotalCount)
			}
			return a.printer("").PrintList(items)
		},
	}
	cmd.Flags().StringVar(&opts.selector, "selector", "", "entity selector (required), e.g. 'type(SERVICE)'")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time for entity observation")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time for entity observation")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of entities")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order (name or -name)")
	cmd.Flags().StringVar(&opts.mz, "mz", "", "management zone selector")
	return withScopes(cmd, "entities.read")
}
