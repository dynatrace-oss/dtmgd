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
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/entities/%s", url.PathEscape(args[0]))
			params := map[string]string{"fields": "+properties,+toRelationships,+fromRelationships,+tags,+managementZones"}

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					var result map[string]interface{}
					if err := c.GetV2(path, params, &result); err != nil {
						return nil, err
					}
					return result, nil
				})
				if err != nil {
					return err
				}
				return a.printer("entity").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := c.GetV2(path, params, &result); err != nil {
				return err
			}

			return a.printer("entity").Print(result)
		},
	}
	return cmd
}

func newDescribeEntityTypeCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "entity-type <type>",
		Aliases: []string{"et"},
		Short:   "Show schema details of an entity type",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/entityTypes/%s", url.PathEscape(args[0]))

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					var result map[string]interface{}
					if err := c.GetV2(path, nil, &result); err != nil {
						return nil, err
					}
					return result, nil
				})
				if err != nil {
					return err
				}
				return a.printer("entityType").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := c.GetV2(path, nil, &result); err != nil {
				return err
			}

			return a.printer("entityType").Print(result)
		},
	}
	return cmd
}

func newDescribeEntityRelationsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "entity-relations <entity-id>",
		Aliases: []string{"entity-rels", "ent-rel"},
		Short:   "Show entity relationships",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/entities/%s", url.PathEscape(args[0]))
			params := map[string]string{"fields": "+toRelationships,+fromRelationships"}

			extractRels := func(result map[string]interface{}) map[string]interface{} {
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

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					var result map[string]interface{}
					if err := c.GetV2(path, params, &result); err != nil {
						return nil, err
					}
					return extractRels(result), nil
				})
				if err != nil {
					return err
				}
				return a.printer("entityRelationships").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := c.GetV2(path, params, &result); err != nil {
				return err
			}

			return a.printer("entityRelationships").Print(extractRels(result))
		},
	}
	return cmd
}
