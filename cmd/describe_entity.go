package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

func newDescribeEntityCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "entity <entity-id>",
		Aliases: []string{"ent"},
		Short:   "Show detailed information about a specific entity",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/entities/%s", url.PathEscape(args[0]))
			params := map[string]string{"fields": "+properties,+toRelationships,+fromRelationships,+tags,+managementZones"}
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "entity",
				Fetch:    describeFetch(path, params),
			})
		},
	}
	return withScopes(cmd, scopeEntitiesRead)
}

func newDescribeEntityTypeCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "entity-type <type>",
		Aliases: []string{"et"},
		Short:   "Show schema details of an entity type",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/entityTypes/%s", url.PathEscape(args[0]))
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "entity-type",
				Fetch:    describeFetch(path, nil),
			})
		},
	}
	return withScopes(cmd, scopeEntitiesRead)
}

func newDescribeEntityRelationsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "entity-relations <entity-id>",
		Aliases: []string{"entity-rels", "ent-rel"},
		Short:   "Show entity relationships",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fetch := describeFetch(fmt.Sprintf("/entities/%s", url.PathEscape(args[0])),
				map[string]string{"fields": "+toRelationships,+fromRelationships"})
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "entity-relations",
				Fetch: func(c *client.Client, limit int) (fetched, error) {
					f, err := fetch(c, limit)
					if err != nil {
						return f, err
					}
					f.Body = extractRels(f.Body.(map[string]interface{}))
					return f, nil
				},
			})
		},
	}
	return withScopes(cmd, scopeEntitiesRead)
}

// extractRels keeps only an entity's relationship keys, or returns the entity
// unchanged when it has none.
func extractRels(result map[string]interface{}) map[string]interface{} {
	relationships := map[string]interface{}{}
	if to, ok := result["toRelationships"]; ok {
		relationships["toRelationships"] = to
	}
	if from, ok := result["fromRelationships"]; ok {
		relationships["fromRelationships"] = from
	}
	if len(relationships) == 0 {
		return result
	}
	return relationships
}
