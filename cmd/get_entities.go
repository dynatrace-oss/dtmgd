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

var (
	entSelector string
	entFrom     string
	entTo       string
	entLimit    int
	entSort     string
	entMZ       string
)

var getEntityTypesCmd = &cobra.Command{
	Use:     "entity-types",
	Aliases: []string{"et", "entity-type"},
	Short:   "List all available entity types",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				raw, err := c.GetV2Paged("/entityTypes", map[string]string{"pageSize": "500"}, maxPages)
				if err != nil {
					return nil, err
				}
				return raw, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("entityTypes").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		raw, err := c.GetV2Paged("/entityTypes", map[string]string{"pageSize": "500"}, maxPages)
		if err != nil {
			return err
		}

		var resp EntityTypesResponse
		if err := client.DecodePaged(raw, &resp); err != nil {
			return err
		}

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("entityTypes").Print(resp)
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
		return NewPrinter().PrintList(items)
	},
}

var getEntitiesCmd = &cobra.Command{
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
		if entSelector == "" {
			return fmt.Errorf("--selector is required (e.g. 'type(SERVICE)')")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		params := map[string]string{
			"entitySelector": entSelector,
		}
		pageSize := 100
		if entLimit > 0 {
			pageSize = entLimit
		}
		params["pageSize"] = fmt.Sprintf("%d", pageSize)
		if entFrom != "" {
			params["from"] = entFrom
		}
		if entTo != "" {
			params["to"] = entTo
		}
		if entSort != "" {
			params["sort"] = entSort
		}
		if entMZ != "" {
			params["mzSelector"] = entMZ
		}

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				raw, err := c.GetV2Paged("/entities", params, effectiveMaxPages(entLimit > 0))
				if err != nil {
					return nil, err
				}
				return raw, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("entities").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		raw, err := c.GetV2Paged("/entities", params, effectiveMaxPages(entLimit > 0))
		if err != nil {
			return err
		}

		var resp EntitiesResponse
		if err := client.DecodePaged(raw, &resp); err != nil {
			return err
		}

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("entities").Print(resp)
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
			output.PrintInfo("No entities found for selector: %s", entSelector)
			return nil
		}
		if resp.TotalCount > len(items) {
			output.PrintInfo("Showing %d of %d entities. Use --limit or a more specific selector.", len(items), resp.TotalCount)
		}
		return NewPrinter().PrintList(items)
	},
}

func init() {
	getCmd.AddCommand(getEntityTypesCmd)
	getCmd.AddCommand(getEntitiesCmd)

	getEntitiesCmd.Flags().StringVar(&entSelector, "selector", "", "entity selector (required), e.g. 'type(SERVICE)'")
	getEntitiesCmd.Flags().StringVar(&entFrom, "from", "", "start time for entity observation")
	getEntitiesCmd.Flags().StringVar(&entTo, "to", "", "end time for entity observation")
	getEntitiesCmd.Flags().IntVar(&entLimit, "limit", 0, "maximum number of entities")
	getEntitiesCmd.Flags().StringVar(&entSort, "sort", "", "sort order (name or -name)")
	getEntitiesCmd.Flags().StringVar(&entMZ, "mz", "", "management zone selector")
}
