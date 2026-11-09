package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

const apiTokensPath = "GET /e/env1/api/v2/apiTokens"

func tokenDate(daysAgo int) string {
	return time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02T15:04:05.000Z")
}

// tokensRoute serves one page of tokens: fresh (used yesterday), stale (used
// 200 days ago), never (created 100 days ago, never used), new (created today,
// never used), expired, off (disabled).
func tokensRoute() map[string]route {
	body := fmt.Sprintf(`{"totalCount":6,"pageSize":10000,"apiTokens":[
{"id":"fresh","name":"fresh","owner":"a","enabled":true,"creationDate":%q,"lastUsedDate":%q,"scopes":["a"]},
{"id":"stale","name":"stale","owner":"a","enabled":true,"creationDate":%q,"lastUsedDate":%q,"scopes":["a","b"]},
{"id":"never","name":"never","owner":"b","enabled":true,"creationDate":%q,"scopes":[]},
{"id":"new","name":"new","owner":"b","enabled":true,"creationDate":%q,"scopes":[]},
{"id":"expired","name":"expired","owner":"c","enabled":true,"creationDate":%q,"lastUsedDate":%q,"expirationDate":%q,"scopes":[]},
{"id":"off","name":"off","owner":"c","enabled":false,"creationDate":%q,"lastUsedDate":%q,"scopes":[]}]}`,
		tokenDate(300), tokenDate(1),
		tokenDate(300), tokenDate(200),
		tokenDate(100),
		tokenDate(0),
		tokenDate(300), tokenDate(5), tokenDate(10),
		tokenDate(300), tokenDate(5))
	return map[string]route{apiTokensPath: {Body: body}}
}

func tokenIDs(t *testing.T, stdout string) []string {
	t.Helper()
	var body struct {
		APITokens []struct {
			ID string `json:"id"`
		} `json:"apiTokens"`
	}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	var ids []string
	for _, tk := range body.APITokens {
		ids = append(ids, tk.ID)
	}
	return ids
}

func TestGetAPITokensFilters(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"all", nil, "fresh,stale,never,new,expired,off"},
		{"unused-days", []string{"--unused-days", "90"}, "stale,never"},
		{"never-used", []string{"--never-used"}, "never,new"},
		{"never-used grace", []string{"--never-used", "--grace-days", "30"}, "never"},
		{"expired", []string{"--expired"}, "expired"},
		{"disabled", []string{"--disabled"}, "off"},
		{"any of several", []string{"--expired", "--disabled"}, "expired,off"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, tokensRoute())})
			r := runCLI(t, cfg, append([]string{"get", "api-tokens", "-o", "json"}, c.args...)...)
			if r.Code != 0 {
				t.Fatalf("exit %d: %s", r.Code, r.Stderr)
			}
			if got := strings.Join(tokenIDs(t, r.Stdout), ","); got != c.want {
				t.Errorf("tokens %q, want %q", got, c.want)
			}
		})
	}
}

func TestGetAPITokensLimitAppliesToMatches(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, tokensRoute())})
	r := runCLI(t, cfg, "get", "api-tokens", "--never-used", "--limit", "1", "-o", "json")
	if r.Code != 0 || strings.Join(tokenIDs(t, r.Stdout), ",") != "never" {
		t.Fatalf("exit %d, %s\n%s", r.Code, r.Stdout, r.Stderr)
	}
}

func TestGetAPITokensRequestAndTable(t *testing.T) {
	api := newFakeAPI(t, tokensRoute())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "api-tokens")
	if r.Code != 0 || !strings.Contains(r.Stdout, "stale") || !strings.Contains(r.Stdout, "never") {
		t.Fatalf("exit %d\n%s\n%s", r.Code, r.Stdout, r.Stderr)
	}
	q := api.Queries(apiTokensPath)
	if len(q) != 1 || !strings.Contains(q[0].Get("fields"), "+lastUsedDate") {
		t.Errorf("query %v does not ask for +lastUsedDate", q)
	}
}

func TestGetAPITokensBadFlags(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, tokensRoute())})
	for _, args := range [][]string{{"--unused-days", "0"}, {"--grace-days", "-1"}} {
		r := runCLI(t, cfg, append([]string{"get", "api-tokens"}, args...)...)
		if r.Code != 2 {
			t.Errorf("%v: exit %d, want 2", args, r.Code)
		}
	}
}

// manyTokensRoute serves n tokens, all used yesterday, on one page.
func manyTokensRoute(n int) map[string]route {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"id":"t%d","name":"t%d","owner":"a","enabled":true,"creationDate":%q,"lastUsedDate":%q}`,
			i, i, tokenDate(300), tokenDate(1))
	}
	body := fmt.Sprintf(`{"totalCount":%d,"pageSize":10000,"apiTokens":[%s]}`, n, strings.Join(items, ","))
	return map[string]route{apiTokensPath: {Body: body}}
}

// /apiTokens answers HTTP 400 to a pageSize below 100, so a smaller limit
// still asks for 100 tokens, and the list is cut to the limit.
func TestGetAPITokensPageSizeFloor(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no limit", nil, "10000"},
		{"limit below the minimum", []string{"--limit", "2"}, "100"},
		{"limit above the minimum", []string{"--limit", "150"}, "150"},
		{"filter", []string{"--limit", "2", "--expired"}, "10000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api := newFakeAPI(t, tokensRoute())
			r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), append([]string{"get", "api-tokens", "-o", "json"}, c.args...)...)
			if r.Code != 0 {
				t.Fatalf("exit %d: %s", r.Code, r.Stderr)
			}
			if q := api.Queries(apiTokensPath); len(q) != 1 || q[0].Get("pageSize") != c.want {
				t.Errorf("queries %v, want one with pageSize=%s", q, c.want)
			}
		})
	}
}

func TestGetAPITokensSmallLimitIsCut(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, tokensRoute())})
	r := runCLI(t, cfg, "get", "api-tokens", "--limit", "2", "-o", "json")
	if got := strings.Join(tokenIDs(t, r.Stdout), ","); r.Code != 0 || got != "fresh,stale" {
		t.Fatalf("exit %d, tokens %q, want fresh,stale\n%s", r.Code, got, r.Stderr)
	}
	r = runCLI(t, cfg, "get", "api-tokens", "--limit", "2")
	wantStderr(t, r, "Showing 2 of 6 api-tokens")
}

func TestGetAPITokensAgentDefaultLimit(t *testing.T) {
	api := newFakeAPI(t, manyTokensRoute(60))
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "api-tokens", "-A")
	if q := api.Queries(apiTokensPath); len(q) != 1 || q[0].Get("pageSize") != "100" {
		t.Errorf("queries %v, want one with pageSize=100", q)
	}
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("%v\n%s", err, r.Stdout)
	}
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tokenIDs(t, string(raw))); n != 50 {
		t.Errorf("%d tokens, want 50", n)
	}
	if resp.Context == nil || !resp.Context.HasMore || resp.Context.Total == nil || *resp.Context.Total != 60 {
		t.Errorf("context = %+v; want has_more and total 60", resp.Context)
	}
}
