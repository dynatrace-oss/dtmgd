package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// apiTokensPageMin and apiTokensPageCap are the smallest and largest pageSize
// /apiTokens accepts; outside them it answers HTTP 400.
const (
	apiTokensPageMin = 100
	apiTokensPageCap = 10000
)

// apiTokensFields are the optional fields get api-tokens asks for: the list
// returns only id, name, enabled, owner and creationDate otherwise.
const apiTokensFields = "+lastUsedDate,+lastUsedIpAddress,+scopes,+expirationDate"

// APITokenListItem is the table row for API tokens.
type APITokenListItem struct {
	ID       string `table:"ID"`
	Name     string `table:"NAME,max=40"`
	Owner    string `table:"OWNER"`
	Enabled  string `table:"ENABLED"`
	Created  string `table:"CREATED"`
	LastUsed string `table:"LAST-USED"`
	Expires  string `table:"EXPIRES"`
	Scopes   int    `table:"SCOPES"`
	LastIP   string `table:"LAST-IP,wide"`
}

// APITokensResponse models /api/v2/apiTokens.
type APITokensResponse struct {
	APITokens  []APITokenEntry `json:"apiTokens"`
	TotalCount int             `json:"totalCount"`
}

type APITokenEntry struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Owner             string   `json:"owner"`
	Enabled           bool     `json:"enabled"`
	CreationDate      string   `json:"creationDate"`
	LastUsedDate      string   `json:"lastUsedDate"`
	LastUsedIPAddress string   `json:"lastUsedIpAddress"`
	ExpirationDate    string   `json:"expirationDate"`
	Scopes            []string `json:"scopes"`
}

// getAPITokensOptions holds the flags of get api-tokens.
type getAPITokensOptions struct {
	selector    string
	sort        string
	limit       int
	unusedDays  int
	neverUsed   bool
	expired     bool
	disabled    bool
	graceDays   int
	unusedGiven bool
}

// filtering reports whether any client-side filter is set.
func (o getAPITokensOptions) filtering() bool {
	return o.unusedGiven || o.neverUsed || o.expired || o.disabled
}

// apiTokensParams is the /apiTokens query for the flags.
func apiTokensParams(opts getAPITokensOptions) map[string]string {
	params := map[string]string{"fields": apiTokensFields}
	if opts.selector != "" {
		params["apiTokenSelector"] = opts.selector
	}
	if opts.sort != "" {
		params["sort"] = opts.sort
	}
	return params
}

// parseTokenDate parses a date the API returns; ok is false for "" or junk.
func parseTokenDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

// uselessReasons names why a token counts as useless at now, or nil when it
// does not. A token is only judged by the filters that are set; with several
// set, matching any one is enough.
func (o getAPITokensOptions) uselessReasons(t APITokenEntry, now time.Time) []string {
	var reasons []string
	last, used := parseTokenDate(t.LastUsedDate)
	created, hasCreated := parseTokenDate(t.CreationDate)
	grace := time.Duration(o.graceDays) * 24 * time.Hour

	if o.neverUsed && !used && (!hasCreated || now.Sub(created) >= grace) {
		reasons = append(reasons, "never-used")
	}
	if o.unusedGiven {
		cutoff := now.Add(-time.Duration(o.unusedDays) * 24 * time.Hour)
		switch {
		case used && last.Before(cutoff):
			reasons = append(reasons, fmt.Sprintf("unused>%dd", o.unusedDays))
		case !used && hasCreated && created.Before(cutoff):
			reasons = append(reasons, fmt.Sprintf("never-used>%dd", o.unusedDays))
		}
	}
	if o.expired {
		if exp, ok := parseTokenDate(t.ExpirationDate); ok && exp.Before(now) {
			reasons = append(reasons, "expired")
		}
	}
	if o.disabled && !t.Enabled {
		reasons = append(reasons, "disabled")
	}
	return reasons
}

func newGetAPITokensCmd(a *App) *cobra.Command {
	var opts getAPITokensOptions
	cmd := &cobra.Command{
		Use:     "api-tokens",
		Args:    noArgs,
		Aliases: []string{"api-token", "tokens"},
		Short:   "List API tokens and find the unused ones",
		Long: `List the environment's API tokens (never their values), with owner, creation,
last use, expiry and scope count.

Filters run on the client, after all tokens are fetched. With several set, a
token matching any one is kept:

  --unused-days N   last used more than N days ago, or never used and older than N days
  --never-used      never used (tokens younger than --grace-days are skipped)
  --expired         expiration date in the past
  --disabled        disabled

--selector is the apiTokenSelector DSL, applied by the API:
  name("..."), owner("..."), enabled(true|false), personalAccessToken(true|false)

lastUsedDate is refreshed by Dynatrace about once an hour.

Needs the token scope apiTokens.read.

Examples:
  dtmgd get api-tokens
  dtmgd get api-tokens --unused-days 90
  dtmgd get api-tokens --never-used --grace-days 14 -o json
  dtmgd get api-tokens --unused-days 90 --expired --disabled
  dtmgd get api-tokens --selector 'owner("jane@example.com")' --sort=-lastUsedDate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.unusedGiven = cmd.Flags().Changed("unused-days")
			if opts.unusedGiven && opts.unusedDays < 1 {
				return &UsageError{Err: errors.New("--unused-days must be at least 1")}
			}
			if opts.graceDays < 0 {
				return &UsageError{Err: errors.New("--grace-days must not be negative")}
			}
			if opts.sort != "" && !strings.HasPrefix(opts.sort, "-") && !strings.HasPrefix(opts.sort, "+") {
				opts.sort = "+" + opts.sort
			}
			base := pagedFetchMin("/apiTokens", apiTokensParams(opts), "apiTokens", apiTokensPageMin, apiTokensPageCap, a.Opts.MaxPages)
			spec := fetchSpec{
				Verb:     "get",
				Resource: "api-tokens",
				Fetch:    filteredTokensFetch(base, opts),
				Rows:     apiTokenRows,
				Link:     uiPage(uiAPITokens),
				Limit:    opts.limit,
				List:     true,
				Empty:    "No API tokens found.",
			}
			return a.runFetch(spec)
		},
	}
	cmd.Flags().StringVar(&opts.selector, "selector", "", "apiTokenSelector DSL to filter tokens (e.g. 'owner(\"jane@example.com\")')")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort by name, owner, creationDate, expirationDate or lastUsedDate; prefix with - to descend")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of results to show")
	cmd.Flags().IntVar(&opts.unusedDays, "unused-days", 0, "keep tokens last used more than N days ago, or never used and older than N days")
	cmd.Flags().BoolVar(&opts.neverUsed, "never-used", false, "keep tokens that were never used")
	cmd.Flags().IntVar(&opts.graceDays, "grace-days", 0, "with --never-used, skip tokens created less than N days ago")
	cmd.Flags().BoolVar(&opts.expired, "expired", false, "keep tokens whose expiration date has passed")
	cmd.Flags().BoolVar(&opts.disabled, "disabled", false, "keep disabled tokens")
	return withScopes(cmd, scopeAPITokensRead)
}

// filteredTokensFetch wraps base with the client-side filters. Without any
// filter it is base. With one, every token is fetched (the filter decides what
// counts), the matches are kept, and limit applies to the matches. Each kept
// token gets a "uselessReasons" list in json, yaml and agent output.
func filteredTokensFetch(base func(*client.Client, int) (fetched, error), opts getAPITokensOptions) func(*client.Client, int) (fetched, error) {
	if !opts.filtering() {
		return base
	}
	return func(c *client.Client, limit int) (fetched, error) {
		f, err := base(c, 0)
		if err != nil {
			return f, err
		}
		body := f.Body.(map[string]interface{})
		all, _ := body["apiTokens"].([]interface{})
		kept, err := opts.keepUseless(all, time.Now().UTC())
		if err != nil {
			return fetched{}, err
		}
		total := len(kept)
		more := false
		if limit > 0 && len(kept) > limit {
			kept = kept[:limit]
			more = true
		}
		body["apiTokens"] = kept
		body["totalCount"] = float64(total)
		f.Body = body
		f.Total = total
		f.HasMore = more
		f.Count = len(kept)
		return f, nil
	}
}

// keepUseless returns the tokens of all that match a filter at now, each
// with its "uselessReasons".
func (o getAPITokensOptions) keepUseless(all []interface{}, now time.Time) ([]interface{}, error) {
	kept := make([]interface{}, 0, len(all))
	for _, raw := range all {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		var entry APITokenEntry
		if err := client.DecodePaged(m, &entry); err != nil {
			return nil, err
		}
		if reasons := o.uselessReasons(entry, now); len(reasons) > 0 {
			m["uselessReasons"] = reasons
			kept = append(kept, m)
		}
	}
	return kept, nil
}

// apiTokenRows turns an /apiTokens body into table rows.
func apiTokenRows(body interface{}) (interface{}, int, error) {
	var resp APITokensResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]APITokenListItem, 0, len(resp.APITokens))
	for _, t := range resp.APITokens {
		enabled := "yes"
		if !t.Enabled {
			enabled = "no"
		}
		items = append(items, APITokenListItem{
			ID:       t.ID,
			Name:     t.Name,
			Owner:    t.Owner,
			Enabled:  enabled,
			Created:  tokenDay(t.CreationDate, "-"),
			LastUsed: tokenDay(t.LastUsedDate, "never"),
			Expires:  tokenDay(t.ExpirationDate, "-"),
			Scopes:   len(t.Scopes),
			LastIP:   t.LastUsedIPAddress,
		})
	}
	return items, len(items), nil
}

// tokenDay is the date part of an API timestamp, or empty for "".
func tokenDay(s, empty string) string {
	if s == "" {
		return empty
	}
	if t, ok := parseTokenDate(s); ok {
		return t.Format("2006-01-02")
	}
	return s
}
