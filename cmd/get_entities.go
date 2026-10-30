package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
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
	var opts struct {
		limit int
	}
	cmd := &cobra.Command{
		Use:     "entity-types",
		Args:    noArgs,
		Aliases: []string{"et", "entity-type"},
		Short:   "List all available entity types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFetch(fetchSpec{
				Verb:     "get",
				Resource: "entity-types",
				Fetch:    pagedFetch("/entityTypes", nil, "types", 500, a.Opts.MaxPages),
				Rows:     entityTypeRows,
				Limit:    opts.limit,
				List:     true,
				Empty:    "No entity types found.",
			})
		},
	}
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of entity types")
	return withScopes(cmd, scopeEntitiesRead)
}

// entityTypeRows turns an /entityTypes body into table rows.
func entityTypeRows(body interface{}) (interface{}, int, error) {
	var resp EntityTypesResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]EntityTypeListItem, 0, len(resp.Types))
	for _, t := range resp.Types {
		items = append(items, EntityTypeListItem{
			Type:        t.Type,
			DisplayName: t.DisplayName,
			Properties:  len(t.Properties),
		})
	}
	return items, len(items), nil
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
		Args:    noArgs,
		Aliases: []string{"ent", "entity"},
		Short:   "Discover entities using an entity selector",
		Long: `Discover monitored entities using the Dynatrace entity selector syntax.
--selector is required and must specify exactly ONE entity type.

Examples:
  dtmgd get entities --selector 'type(SERVICE)'
  dtmgd get entities --selector 'type(HOST),healthState("HEALTHY")'
  dtmgd get entities --selector 'type(SERVICE)' --mz Production
  dtmgd get entities --env ALL_ENVIRONMENTS --selector 'type(SERVICE)'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.selector == "" {
				return &UsageError{Err: fmt.Errorf("--selector is required (e.g. 'type(SERVICE)')")}
			}

			// The API ignores an mzSelector parameter; the zone goes in
			// entitySelector.
			selector := opts.selector
			if opts.mz != "" {
				selector = joinSelector(selector, mzCriterion(opts.mz))
			}
			params := map[string]string{
				"entitySelector": selector,
			}
			fetch := pagedFetch("/entities", params, "entities", 100, a.Opts.MaxPages)
			if opts.mz != "" {
				fetch = hintOnBadRequest(fetch, `A zone name must match exactly: it is case-sensitive, and each environment has its own zones. Check that the zone exists in this environment, or pass mzId(<id>).`)
			}
			if opts.from != "" {
				params["from"] = opts.from
			}
			if opts.to != "" {
				params["to"] = opts.to
			}
			if opts.sort != "" {
				params["sort"] = opts.sort
			}

			return a.runFetch(fetchSpec{
				Verb:     "get",
				Resource: "entities",
				Fetch:    fetch,
				Rows:     entityRows,
				Limit:    opts.limit,
				List:     true,
				Empty:    fmt.Sprintf("No entities found for selector: %s", selector),
			})
		},
	}
	cmd.Flags().StringVar(&opts.selector, "selector", "", "entity selector (required), e.g. 'type(SERVICE)'")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time for entity observation")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time for entity observation")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of entities")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order (name or -name)")
	cmd.Flags().StringVar(&opts.mz, "mz", "", `management zone: a name, or an mzName("…") or mzId(…) criterion, added to --selector`)
	return withScopes(cmd, scopeEntitiesRead)
}

// entityRows turns an /entities body into table rows.
func entityRows(body interface{}) (interface{}, int, error) {
	var resp EntitiesResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]EntityListItem, 0, len(resp.Entities))
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
	return items, len(items), nil
}
