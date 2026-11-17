package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
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
