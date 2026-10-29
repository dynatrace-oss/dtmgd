package cmd

import (
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// The values get environments' --state and --sort accept, as the Cluster API
// spells them.
var (
	environmentStates = []string{"ENABLED", "DISABLED"}
	environmentSorts  = []string{"name", "-name", "creationDate", "-creationDate"}
)

// environmentsMoved is get environments' extra suggestion on an environment
// context, where it used to run the connectivity check doctor now runs.
const environmentsMoved = `"get environments" now lists a cluster's environments. The per-context connectivity check it used to run is: dtmgd doctor`

// clusterEnvironment is the part of a Cluster API environment the tables
// show. Quotas are present only with includeConsumptionInfo, and a quota or
// limit the API leaves null is unknown or unlimited.
type clusterEnvironment struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	State        string   `json:"state"`
	Trial        bool     `json:"trial"`
	CreationDate string   `json:"creationDate"`
	Tags         []string `json:"tags"`
	Quotas       *struct {
		HostUnits *struct {
			CurrentUsage *float64 `json:"currentUsage"`
			MaxLimit     *float64 `json:"maxLimit"`
		} `json:"hostUnits"`
		DavisDataUnits *struct {
			ConsumedThisYear *float64 `json:"consumedThisYear"`
			AnnualLimit      *float64 `json:"annualLimit"`
		} `json:"davisDataUnits"`
	} `json:"quotas"`
}

// environmentRow is a get environments table row.
type environmentRow struct {
	ID      string `table:"ID"`
	Name    string `table:"NAME"`
	State   string `table:"STATE"`
	Trial   string `table:"TRIAL"`
	Created string `table:"CREATED"`
	Tags    string `table:"TAGS,wide"`
}

// environmentUsageRow is a get environments --consumption table row.
type environmentUsageRow struct {
	ID        string `table:"ID"`
	Name      string `table:"NAME"`
	State     string `table:"STATE"`
	Trial     string `table:"TRIAL"`
	Created   string `table:"CREATED"`
	HostUnits string `table:"HOST-UNITS"`
	DDUs      string `table:"DDU-YEAR"`
	Tags      string `table:"TAGS,wide"`
}

func newGetEnvironmentsCmd(a *App) *cobra.Command {
	var opts struct {
		name, state, filter, sort string
		consumption               bool
		limit                     int
	}
	cmd := &cobra.Command{
		Use:     "environments",
		Args:    noArgs,
		Aliases: []string{"envs", "env"},
		Short:   "List the environments of a Dynatrace Managed cluster (cluster context)",
		Long: `List the environments of a cluster, from the Cluster Management API. Needs a
cluster context: one without env-id, whose token has the ServiceProviderAPI
scope.

"get environments" used to check each configured context's connectivity;
that check is now "dtmgd doctor".

Filters combine; an environment must match all of them:
  --name     part of the environment name, case-insensitive
  --state    ENABLED or DISABLED
  --filter   a raw API filter, e.g. 'trial(true)', 'tag(prod)', or
             'tag(a,b)' for either tag

--consumption adds host-unit and yearly DDU usage, as usage / limit. It is
not available on a cluster licensed with the Dynatrace Platform Subscription
(DPS).

Examples:
  dtmgd get environments --context prod-cluster
  dtmgd get environments --state enabled --name shop
  dtmgd get environments --consumption -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := environmentParams(opts.name, opts.state, opts.filter, opts.sort)
			if err != nil {
				return err
			}
			if opts.consumption {
				params["includeConsumptionInfo"] = "true"
			}
			fetch := clusterPagedFetch("/cluster/v2/environments", params, "environments", 1000, a.Opts.MaxPages)
			if opts.consumption {
				fetch = hintOnStatus(fetch, 422, hintDPSConsumption)
			}
			return a.runClusterFetch(fetchSpec{
				Verb:         "get",
				Resource:     "environments",
				Fetch:        fetch,
				Rows:         environmentRows(opts.consumption),
				Limit:        opts.limit,
				List:         true,
				Empty:        "No environments match.",
				WrongContext: environmentsMoved,
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.name, "name", "", "only environments whose name contains this (case-insensitive)")
	f.StringVar(&opts.state, "state", "", "only environments in this state: ENABLED or DISABLED")
	f.StringVar(&opts.filter, "filter", "", "raw API filter, e.g. 'tag(prod)' or 'trial(false)'")
	f.StringVar(&opts.sort, "sort", "", "sort order: name, -name, creationDate or -creationDate")
	f.BoolVar(&opts.consumption, "consumption", false, "include host-unit and DDU consumption (not on DPS-licensed clusters)")
	f.IntVar(&opts.limit, "limit", 0, "maximum number of environments to return")
	return clusterCommand(cmd)
}

// environmentParams validates get environments' filter flags and turns them
// into query parameters: name, state and the raw filter joined with commas
// into filter, and sort.
func environmentParams(name, state, raw, sortBy string) (map[string]string, error) {
	if strings.ContainsAny(name, ",()") {
		return nil, &UsageError{Err: errors.New("--name cannot contain ',', '(' or ')'; use --filter")}
	}
	state, err := enumFlag("state", state, environmentStates, true)
	if err != nil {
		return nil, err
	}
	sortBy, err = enumFlag("sort", sortBy, environmentSorts, false)
	if err != nil {
		return nil, err
	}
	var filter []string
	if name != "" {
		filter = append(filter, "name("+name+")")
	}
	if state != "" {
		filter = append(filter, "state("+state+")")
	}
	if raw != "" {
		filter = append(filter, raw)
	}
	params := map[string]string{}
	if len(filter) > 0 {
		params["filter"] = strings.Join(filter, ",")
	}
	if sortBy != "" {
		params["sort"] = sortBy
	}
	return params, nil
}

// environmentRows is the Rows of get environments, with the consumption
// columns when usage is set.
func environmentRows(usage bool) func(body interface{}) (interface{}, int, error) {
	return func(body interface{}) (interface{}, int, error) {
		var list struct {
			Environments []clusterEnvironment `json:"environments"`
		}
		if err := client.DecodePaged(body.(map[string]interface{}), &list); err != nil {
			return nil, 0, err
		}
		if usage {
			rows := make([]environmentUsageRow, len(list.Environments))
			for i, e := range list.Environments {
				rows[i] = environmentUsageRowOf(e)
			}
			return rows, len(rows), nil
		}
		rows := make([]environmentRow, len(list.Environments))
		for i, e := range list.Environments {
			rows[i] = environmentRowOf(e)
		}
		return rows, len(rows), nil
	}
}

func environmentRowOf(e clusterEnvironment) environmentRow {
	return environmentRow{
		ID:      e.ID,
		Name:    e.Name,
		State:   e.State,
		Trial:   yesNo(e.Trial),
		Created: e.CreationDate,
		Tags:    strings.Join(e.Tags, ", "),
	}
}

func environmentUsageRowOf(e clusterEnvironment) environmentUsageRow {
	r := environmentRowOf(e)
	u := environmentUsageRow{ID: r.ID, Name: r.Name, State: r.State, Trial: r.Trial, Created: r.Created, Tags: r.Tags, HostUnits: "—", DDUs: "—"}
	if e.Quotas == nil {
		return u
	}
	if hu := e.Quotas.HostUnits; hu != nil {
		u.HostUnits = usageOf(hu.CurrentUsage, hu.MaxLimit)
	}
	if ddu := e.Quotas.DavisDataUnits; ddu != nil {
		u.DDUs = usageOf(ddu.ConsumedThisYear, ddu.AnnualLimit)
	}
	return u
}

// usageOf is "usage / limit", with "unlimited" for a limit the API leaves
// null, or "—" when the usage itself is unknown.
func usageOf(usage, limit *float64) string {
	if usage == nil {
		return "—"
	}
	l := "unlimited"
	if limit != nil {
		l = formatNumber(*limit)
	}
	return formatNumber(*usage) + " / " + l
}

// formatNumber prints v with as many digits as it needs and no exponent.
func formatNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// yesNo is "yes" for true and "no" for false.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
