package cmd

import (
	"bytes"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// TestIsSingleValueResult verifies the compact-table trigger for resolution=Inf results.
func TestIsSingleValueResult(t *testing.T) {
	tests := []struct {
		name string
		data []MetricQueryDataPoints
		want bool
	}{
		{
			name: "empty slice",
			data: nil,
			want: false,
		},
		{
			name: "single series with one timestamp",
			data: []MetricQueryDataPoints{
				{Timestamps: []int64{1704067200000}, Values: []*float64{fptr(42.5)}},
			},
			want: true,
		},
		{
			name: "multiple series all with one timestamp",
			data: []MetricQueryDataPoints{
				{Timestamps: []int64{1704067200000}, Values: []*float64{fptr(10.0)}},
				{Timestamps: []int64{1704067200000}, Values: []*float64{fptr(20.0)}},
				{Timestamps: []int64{1704067200000}, Values: []*float64{fptr(30.0)}},
			},
			want: true,
		},
		{
			name: "single series with multiple timestamps",
			data: []MetricQueryDataPoints{
				{Timestamps: []int64{1000, 2000, 3000}, Values: []*float64{fptr(1), fptr(2), fptr(3)}},
			},
			want: false,
		},
		{
			name: "mixed: first single, second multi",
			data: []MetricQueryDataPoints{
				{Timestamps: []int64{1000}, Values: []*float64{fptr(1.0)}},
				{Timestamps: []int64{1000, 2000}, Values: []*float64{fptr(1.0), fptr(2.0)}},
			},
			want: false,
		},
		{
			name: "series with no timestamps",
			data: []MetricQueryDataPoints{
				{Timestamps: nil, Values: nil},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSingleValueResult(tt.data)
			if got != tt.want {
				t.Errorf("isSingleValueResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractEntityLabel verifies entity name/ID extraction from dimension maps.
func TestExtractEntityLabel(t *testing.T) {
	tests := []struct {
		name     string
		dimMap   map[string]string
		wantName string
		wantID   string
	}{
		{
			name: "service entity with name",
			dimMap: map[string]string{
				"dt.entity.service":      "SERVICE-ABC123",
				"dt.entity.service.name": "payments",
			},
			wantName: "payments",
			wantID:   "SERVICE-ABC123",
		},
		{
			name: "host entity with name",
			dimMap: map[string]string{
				"dt.entity.host":      "HOST-DEADBEEF",
				"dt.entity.host.name": "web-server-1",
			},
			wantName: "web-server-1",
			wantID:   "HOST-DEADBEEF",
		},
		{
			name: "entity without name field",
			dimMap: map[string]string{
				"dt.entity.service": "SERVICE-NNNNN",
			},
			wantName: "",
			wantID:   "SERVICE-NNNNN",
		},
		{
			name: "no dt.entity prefix — fallback to first value",
			dimMap: map[string]string{
				"customDim": "some-value",
			},
			wantName: "",
			wantID:   "some-value",
		},
		{
			name:     "empty map",
			dimMap:   map[string]string{},
			wantName: "",
			wantID:   "",
		},
		{
			name: "only .name field (ends with .name, no entity key)",
			dimMap: map[string]string{
				"dt.entity.service.name": "payments",
			},
			// .name suffix is skipped; no bare dt.entity.X found → fallback to first value
			wantName: "",
			wantID:   "payments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotID := extractEntityLabel(tt.dimMap)
			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
			if gotID != tt.wantID {
				t.Errorf("id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}

// fptr is a helper to create a *float64 from a float64 literal.
func fptr(v float64) *float64 { return &v }

// TestInjectEntityNames verifies that injectEntityNames populates "dt.entity.X.name"
// keys in DimensionMaps from a pre-built name map.  This covers the bug fix that
// resolved blank ENTITY columns in the metrics table output.
func TestInjectEntityNames(t *testing.T) {
	resp := &MetricQueryResponse{
		Result: []MetricQueryResult{
			{
				MetricID: "builtin:service.requestCount.total",
				Data: []MetricQueryDataPoints{
					{
						DimensionMap: map[string]string{
							"dt.entity.service": "SERVICE-ABC123",
						},
					},
					{
						DimensionMap: map[string]string{
							"dt.entity.service": "SERVICE-DEF456",
						},
					},
				},
			},
			{
				MetricID: "builtin:service.errors.total.count",
				Data: []MetricQueryDataPoints{
					{
						DimensionMap: map[string]string{
							"dt.entity.service": "SERVICE-ABC123",
						},
					},
					{
						// Entity not in the name map — should remain without .name key.
						DimensionMap: map[string]string{
							"dt.entity.service": "SERVICE-UNKNOWN",
						},
					},
				},
			},
		},
	}

	nameMap := map[string]string{
		"SERVICE-ABC123": "payments",
		"SERVICE-DEF456": "orders",
	}

	injectEntityNames(resp, nameMap)

	tests := []struct {
		ri, di   int
		wantName string
		wantID   string
	}{
		{0, 0, "payments", "SERVICE-ABC123"},
		{0, 1, "orders", "SERVICE-DEF456"},
		{1, 0, "payments", "SERVICE-ABC123"},
		{1, 1, "", "SERVICE-UNKNOWN"}, // unknown entity: name must be absent
	}

	for _, tt := range tests {
		dm := resp.Result[tt.ri].Data[tt.di].DimensionMap
		name, id := extractEntityLabel(dm)
		if name != tt.wantName {
			t.Errorf("result[%d].data[%d]: name = %q, want %q", tt.ri, tt.di, name, tt.wantName)
		}
		if id != tt.wantID {
			t.Errorf("result[%d].data[%d]: id = %q, want %q", tt.ri, tt.di, id, tt.wantID)
		}
	}
}

// TestInjectEntityNames_NoOp verifies that an empty name map leaves DimensionMaps unchanged.
func TestInjectEntityNames_NoOp(t *testing.T) {
	resp := &MetricQueryResponse{
		Result: []MetricQueryResult{
			{
				Data: []MetricQueryDataPoints{
					{DimensionMap: map[string]string{"dt.entity.service": "SERVICE-XYZ"}},
				},
			},
		},
	}

	injectEntityNames(resp, map[string]string{})

	dm := resp.Result[0].Data[0].DimensionMap
	if _, hasName := dm["dt.entity.service.name"]; hasName {
		t.Error("expected no .name key injected when nameMap is empty")
	}
}

// TestPrintTimeSeriesDataPoint exercises all label-derivation branches of the function
// (name+id, id-only, dimensions list, dimensionMap fallback, empty).
func TestPrintTimeSeriesDataPoint(t *testing.T) {
	v42 := fptr(42.0)
	cases := []struct {
		dp    MetricQueryDataPoints
		label string // "" = no Entity line
	}{
		{
			// name + id resolved
			MetricQueryDataPoints{
				DimensionMap: map[string]string{
					"dt.entity.service":      "SERVICE-ABC",
					"dt.entity.service.name": "payments",
				},
				Timestamps: []int64{1704067200000, 1704070800000},
				Values:     []*float64{v42, nil},
			},
			"payments (SERVICE-ABC)",
		},
		{
			// id only (no name)
			MetricQueryDataPoints{
				DimensionMap: map[string]string{"dt.entity.service": "SERVICE-XYZ"},
				Timestamps:   []int64{1704067200000},
				Values:       []*float64{v42},
			},
			"SERVICE-XYZ",
		},
		{
			// Dimensions slice used when DimensionMap is empty
			MetricQueryDataPoints{Dimensions: []string{"prod", "eu"}},
			"[prod eu]",
		},
		{
			// Completely empty input
			MetricQueryDataPoints{DimensionMap: map[string]string{}},
			"",
		},
	}

	for _, tc := range cases {
		var buf bytes.Buffer
		printTimeSeriesDataPoint(&buf, tc.dp)
		out := buf.String()
		if tc.label != "" && !strings.Contains(out, "  Entity: "+tc.label+"\n") {
			t.Errorf("label %q missing:\n%s", tc.label, out)
		}
		if tc.label == "" && strings.Contains(out, "Entity:") {
			t.Errorf("unexpected Entity line:\n%s", out)
		}
		if !strings.Contains(out, "TIMESTAMP (UTC)") {
			t.Errorf("no header:\n%s", out)
		}
	}

	var buf bytes.Buffer
	printTimeSeriesDataPoint(&buf, cases[0].dp)
	for _, want := range []string{"2024-01-01 00:00:00         42.0000", "2024-01-01 01:00:00         null"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, buf.String())
		}
	}
}

// TestPrintEntitySummaryTable pins the sort: value descending, nulls last.
func TestPrintEntitySummaryTable(t *testing.T) {
	a := newTestApp("")
	v1, v2 := fptr(100.0), fptr(50.0)
	data := []MetricQueryDataPoints{
		{
			DimensionMap: map[string]string{
				"dt.entity.service":      "SERVICE-B",
				"dt.entity.service.name": "orders",
			},
			Timestamps: []int64{1704067200000},
			Values:     []*float64{v2},
		},
		{
			// null value entry
			DimensionMap: map[string]string{"dt.entity.service": "SERVICE-C"},
			Timestamps:   []int64{1704067200000},
			Values:       []*float64{nil},
		},
		{
			DimensionMap: map[string]string{
				"dt.entity.service":      "SERVICE-A",
				"dt.entity.service.name": "payments",
			},
			Timestamps: []int64{1704067200000},
			Values:     []*float64{v1},
		},
	}

	var buf bytes.Buffer
	if err := a.printEntitySummaryTable(&buf, data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	a1, b1, c1 := strings.Index(out, "SERVICE-A"), strings.Index(out, "SERVICE-B"), strings.Index(out, "SERVICE-C")
	if a1 < 0 || b1 < 0 || c1 < 0 || a1 > b1 || b1 > c1 {
		t.Errorf("want SERVICE-A (100), SERVICE-B (50), SERVICE-C (null) in that order:\n%s", out)
	}
	if !strings.Contains(out, "100.00") || !strings.Contains(out, "null") {
		t.Errorf("values missing:\n%s", out)
	}
}

// TestPrintMetricQuerySummary pins that the whole summary, headings
// included, is written to the given writer (stdout), for both the
// single-value and time-series branches.
func TestPrintMetricQuerySummary(t *testing.T) {
	a := newTestApp("")
	v := fptr(1.5)
	resp := MetricQueryResponse{
		Resolution: "1h",
		Result: []MetricQueryResult{
			{
				MetricID: "builtin:service.requestCount.total",
				Data: []MetricQueryDataPoints{
					{
						// single-value branch
						DimensionMap: map[string]string{"dt.entity.service": "SERVICE-A"},
						Timestamps:   []int64{1704067200000},
						Values:       []*float64{v},
					},
				},
			},
			{
				MetricID: "builtin:service.errors.total.count",
				Data: []MetricQueryDataPoints{
					{
						// time-series branch (multiple timestamps)
						DimensionMap: map[string]string{"dt.entity.service": "SERVICE-B"},
						Timestamps:   []int64{1704067200000, 1704070800000},
						Values:       []*float64{v, v},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := a.printMetricQuerySummary(&buf, resp); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Resolution: 1h",
		"Metric: builtin:service.requestCount.total",
		"SERVICE-A",
		"Metric: builtin:service.errors.total.count",
		"Entity: SERVICE-B",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, buf.String())
		}
	}
}

// TestQueryMetricsFlagChecksAndErrors pins query metrics' usage errors,
// checked before any request and --metric first, and how a failed or
// undecodable /metrics/query response is reported.
func TestQueryMetricsFlagChecksAndErrors(t *testing.T) {
	metricRequired := "== exit code: 2\n== stdout\n== stderr\nError: --metric is required (e.g. builtin:service.response.time)\n"
	query := []string{"--metric", "builtin:host.cpu.usage", "--from", "now-1h"}
	cases := []struct {
		name  string
		route route
		args  []string
		hits  int // /metrics/query requests sent
		want  string
	}{
		{"neither flag", route{Body: metricQueryBody}, nil, 0, metricRequired},
		{"no --metric", route{Body: metricQueryBody}, []string{"--from", "now-1h"}, 0, metricRequired},
		{"no --from", route{Body: metricQueryBody}, []string{"--metric", "m"}, 0,
			"== exit code: 2\n== stdout\n== stderr\nError: --from is required (e.g. now-1h)\n"},
		{"API error", route{Status: 403, File: "error_403.json"}, query, 1, "== exit code: 5\n" +
			"== stdout\n" +
			"== stderr\n" +
			"Error: API error 403: Token is missing required scope\n" +
			"Hint: Permission denied. Your API token may be missing required scopes.\n" +
			"  This command needs the token scope(s): metrics.read, entities.read.\n"},
	}
	for _, tc := range cases {
		api := newFakeAPI(t, map[string]route{metricsQueryPath: tc.route})
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), append([]string{"query", "metrics"}, tc.args...)...)
		if got := r.String(); got != tc.want {
			t.Errorf("%s:\n--- want\n%s\n--- got\n%s", tc.name, tc.want, got)
		}
		if got := api.Hits(metricsQueryPath); got != tc.hits {
			t.Errorf("%s: %d requests, want %d", tc.name, got, tc.hits)
		}
	}

	// A body the table cannot decode fails the command; nothing is printed
	// and no entity lookup is made.
	api := newFakeAPI(t, map[string]route{metricsQueryPath: {Body: `{"resolution":5}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), append([]string{"query", "metrics"}, query...)...)
	if r.Code != 1 || r.Stdout != "" || !strings.HasPrefix(r.Stderr, "Error: json: cannot unmarshal number") || api.Hits(entitiesPath) != 0 {
		t.Errorf("undecodable body: code %d, stdout %q, stderr %q, %d entity lookups", r.Code, r.Stdout, r.Stderr, api.Hits(entitiesPath))
	}
}

// TestQueryMetricsQueryParams pins the /metrics/query query (--to defaults to
// now; --resolution and --entity are sent only when given), and that only a
// table pays for the entity-name lookup.
func TestQueryMetricsQueryParams(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    url.Values
		lookups int // /entities requests sent
	}{
		{"defaults", nil, url.Values{"metricSelector": {"builtin:host.cpu.usage"}, "from": {"now-1h"}, "to": {"now"}}, 1},
		{"every flag", []string{"--to", "now-5m", "--resolution", "5m", "--entity", "type(HOST)"}, url.Values{
			"metricSelector": {"builtin:host.cpu.usage"}, "from": {"now-1h"}, "to": {"now-5m"},
			"resolution": {"5m"}, "entitySelector": {"type(HOST)"},
		}, 1},
		{"json", []string{"-o", "json"}, url.Values{"metricSelector": {"builtin:host.cpu.usage"}, "from": {"now-1h"}, "to": {"now"}}, 0},
		{"agent", []string{"-A"}, url.Values{"metricSelector": {"builtin:host.cpu.usage"}, "from": {"now-1h"}, "to": {"now"}}, 0},
	}
	for _, tc := range cases {
		api := newFakeAPI(t, map[string]route{
			metricsQueryPath: {Body: metricQueryBody},
			entitiesPath:     {Body: `{"entities":[{"entityId":"HOST-1","displayName":"web-1"}]}`},
		})
		args := append([]string{"query", "metrics", "--metric", "builtin:host.cpu.usage", "--from", "now-1h"}, tc.args...)
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
		qs := api.Queries(metricsQueryPath)
		if r.Code != 0 || len(qs) != 1 || !reflect.DeepEqual(qs[0], tc.want) {
			t.Errorf("%s: code %d, queries %v, want one with %v", tc.name, r.Code, qs, tc.want)
		}
		if got := api.Hits(entitiesPath); got != tc.lookups {
			t.Errorf("%s: %d entity lookups, want %d", tc.name, got, tc.lookups)
		}
	}
}

// TestInjectEntityNamesOnlyEntityIDKeys pins that names are injected for
// dt.entity.X keys alone: not for other dimensions, and not for an existing
// .name key, even when their value matches a looked-up ID.
func TestInjectEntityNamesOnlyEntityIDKeys(t *testing.T) {
	resp := &MetricQueryResponse{Result: []MetricQueryResult{{Data: []MetricQueryDataPoints{
		{DimensionMap: map[string]string{
			"dt.entity.service":      "SERVICE-1",
			"dt.entity.service.name": "SERVICE-1",
			"region":                 "SERVICE-1",
		}},
		{DimensionMap: nil},
	}}}}
	injectEntityNames(resp, map[string]string{"SERVICE-1": "payments"})
	want := map[string]string{
		"dt.entity.service":      "SERVICE-1",
		"dt.entity.service.name": "payments",
		"region":                 "SERVICE-1",
	}
	if got := resp.Result[0].Data[0].DimensionMap; !reflect.DeepEqual(got, want) {
		t.Errorf("DimensionMap = %v, want %v", got, want)
	}
	if got := resp.Result[0].Data[1].DimensionMap; got != nil {
		t.Errorf("nil DimensionMap became %v", got)
	}
}

// TestPrintEntitySummaryTableTies pins the summary table's exact output and
// its tie-breaks: equal values sort by name, then by ID; nulls (including a
// series without exactly one value) sort last, by the same tie-breaks.
func TestPrintEntitySummaryTableTies(t *testing.T) {
	a := newTestApp("")
	row := func(id, name string, values ...*float64) MetricQueryDataPoints {
		dm := map[string]string{"dt.entity.service": id}
		if name != "" {
			dm["dt.entity.service.name"] = name
		}
		return MetricQueryDataPoints{DimensionMap: dm, Timestamps: []int64{1704067200000}, Values: values}
	}
	data := []MetricQueryDataPoints{
		row("SERVICE-5", "zeta", nil),
		row("SERVICE-4", "beta", fptr(1)),
		row("SERVICE-3", "alpha", fptr(1)),
		row("SERVICE-2", "alpha", fptr(1)),
		row("SERVICE-6", "eta"),
		row("SERVICE-7", "", fptr(2.345)),
		row("SERVICE-8", "eta", fptr(1), fptr(2)),
	}
	var buf bytes.Buffer
	if err := a.printEntitySummaryTable(&buf, data); err != nil {
		t.Fatal(err)
	}
	want := " ENTITY  ENTITY-ID  VALUE \n" +
		"         SERVICE-7  2.35  \n" +
		" alpha   SERVICE-2  1.00  \n" +
		" alpha   SERVICE-3  1.00  \n" +
		" beta    SERVICE-4  1.00  \n" +
		" eta     SERVICE-6  null  \n" +
		" eta     SERVICE-8  null  \n" +
		" zeta    SERVICE-5  null  \n"
	if got := buf.String(); got != want {
		t.Errorf("--- want\n%s\n--- got\n%s", want, got)
	}
}
