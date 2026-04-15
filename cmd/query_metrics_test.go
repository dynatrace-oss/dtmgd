package cmd

import (
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
// (name+id, id-only, dimensions list, dimensionMap fallback, empty).  The function
// writes to stdout; we just verify it doesn't panic with any combination of inputs.
func TestPrintTimeSeriesDataPoint(t *testing.T) {
	v42 := fptr(42.0)
	cases := []MetricQueryDataPoints{
		{
			// name + id resolved
			DimensionMap: map[string]string{
				"dt.entity.service":      "SERVICE-ABC",
				"dt.entity.service.name": "payments",
			},
			Timestamps: []int64{1704067200000, 1704070800000},
			Values:     []*float64{v42, nil},
		},
		{
			// id only (no name)
			DimensionMap: map[string]string{"dt.entity.service": "SERVICE-XYZ"},
			Timestamps:   []int64{1704067200000},
			Values:       []*float64{v42},
		},
		{
			// Dimensions slice used when DimensionMap is empty
			Dimensions: []string{"prod", "eu"},
			Timestamps: []int64{},
			Values:     nil,
		},
		{
			// DimensionMap with non-entity keys (fallback)
			DimensionMap: map[string]string{"host": "web-1", "region": "eu-west"},
			Timestamps:   []int64{1704067200000},
			Values:       []*float64{fptr(1.0)},
		},
		{
			// Completely empty input
			DimensionMap: map[string]string{},
		},
	}

	for _, dp := range cases {
		// Should not panic.
		printTimeSeriesDataPoint(dp)
	}
}

// TestPrintEntitySummaryTable exercises the sort and formatting logic.  Output goes
// to stdout; we verify the function completes without panic for various inputs.
func TestPrintEntitySummaryTable(t *testing.T) {
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
			DimensionMap: map[string]string{
				"dt.entity.service":      "SERVICE-A",
				"dt.entity.service.name": "payments",
			},
			Timestamps: []int64{1704067200000},
			Values:     []*float64{v1},
		},
		{
			// null value entry
			DimensionMap: map[string]string{"dt.entity.service": "SERVICE-C"},
			Timestamps:   []int64{1704067200000},
			Values:       []*float64{nil},
		},
	}

	// Should not panic.
	printEntitySummaryTable(data)
	// Empty slice.
	printEntitySummaryTable(nil)
}

// TestPrintMetricQuerySummary exercises both the single-value and time-series
// branches of the summary printer.
func TestPrintMetricQuerySummary(t *testing.T) {
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

	// Should not panic.
	printMetricQuerySummary(resp)
}
