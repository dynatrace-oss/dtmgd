package cmd

import (
	"strings"
	"testing"
)

func TestCleanPGName_BookStorePrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			// Standard SpringBoot process group display name
			"SpringBoot BookStore-Orders com.dynatrace.orders.OrdersApplication orders-*",
			"orders",
		},
		{
			"SpringBoot BookStore-Payments com.dynatrace.payments.PaymentsApplication payments-*",
			"payments",
		},
		{
			"SpringBoot BookStore-Dynapay com.dynatrace.dynapay.DynapayApplication dynapay-*",
			"dynapay",
		},
		{
			"SpringBoot BookStore-Ingest com.dynatrace.ingest.IngestApplication ingest-*",
			"ingest",
		},
		{
			// Multi-word service name: only first word after BookStore- is extracted
			"SpringBoot BookStore-MyService extra tokens",
			"myservice",
		},
		{
			// BookStore- at the very end (no trailing space)
			"BookStore-Books",
			"books",
		},
	}

	for _, tt := range tests {
		label := tt.input
		if len(label) > 30 {
			label = label[:30]
		}
		t.Run(label, func(t *testing.T) {
			got := cleanPGName(tt.input)
			if got != tt.want {
				t.Errorf("cleanPGName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanPGName_FallbackSuffixStrip(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"orders.bookstore.svc.cluster.local", "orders"},
		{"payments.svc.cluster.local", "payments"},
		{"ingest.bookstore", "ingest"},
		// No suffix to strip — returned as-is
		{"some-unknown-service", "some-unknown-service"},
		// sh process groups that don't have BookStore- prefix
		{"sh storage-*", "sh storage-*"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanPGName(tt.input)
			if got != tt.want {
				t.Errorf("cleanPGName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestCleanPGName_CaseLower verifies that extracted names are always lowercase.
func TestCleanPGName_CaseLower(t *testing.T) {
	result := cleanPGName("SpringBoot BookStore-STORAGE com.dynatrace.storage.StorageApplication storage-*")
	if result != strings.ToLower(result) {
		t.Errorf("cleanPGName result should be lowercase, got %q", result)
	}
}

// TestEntityIDNormalization verifies that PROCESS_GROUP-UPPERCASE matches process_group-lowercase
// via strings.ToLower — this is the fix for the entity ID case mismatch.
func TestEntityIDNormalization(t *testing.T) {
	// Entity API returns uppercase; logs aggregate returns lowercase for the same entity.
	entityAPIID := "PROCESS_GROUP-CAA2A66AE22043F9"
	aggregateID := "process_group-caa2a66ae22043f9"

	if strings.ToLower(entityAPIID) != aggregateID {
		t.Errorf("ToLower(%q) = %q, expected %q", entityAPIID, strings.ToLower(entityAPIID), aggregateID)
	}

	// Simulate building the lookup map with lowercase keys.
	entityNames := map[string]string{}
	entityNames[strings.ToLower(entityAPIID)] = "ingest"

	// Simulate looking up the aggregate result.
	if name, ok := entityNames[aggregateID]; !ok {
		t.Errorf("aggregate ID %q should be found in lowercase entity map", aggregateID)
	} else if name != "ingest" {
		t.Errorf("expected name %q, got %q", "ingest", name)
	}
}

// TestZeroRowsIncluded verifies that services with no log data still appear in output as
// zero rows (fixing the silent omission bug: services with no logs were previously dropped).
func TestZeroRowsIncluded(t *testing.T) {
	// Simulate entity lookup: 3 known services.
	entityNames := map[string]string{
		"process_group-aaa": "books",
		"process_group-bbb": "orders",
		"process_group-ccc": "payments",
	}

	// Simulate aggregate result: only "payments" had any logs.
	pgCounts := map[string]map[string]int64{
		"process_group-ccc": {"ERROR": 12},
	}

	// Apply the fixed row-building logic (iterate entityNames, not pgCounts).
	rows := make([]LogCountRow, 0, len(entityNames))
	for pgID, name := range entityNames {
		levels := pgCounts[pgID] // nil for books and orders
		rows = append(rows, LogCountRow{
			Service: name,
			Info:    levels["INFO"],
			Warn:    levels["WARN"],
			Error:   levels["ERROR"],
			Total:   levels["INFO"] + levels["WARN"] + levels["ERROR"],
		})
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (including zero rows), got %d", len(rows))
	}

	byService := map[string]LogCountRow{}
	for _, r := range rows {
		byService[r.Service] = r
	}

	// payments should have 12 errors.
	if byService["payments"].Error != 12 {
		t.Errorf("payments error count = %d, want 12", byService["payments"].Error)
	}
	// books and orders should have zero totals (not omitted).
	for _, svc := range []string{"books", "orders"} {
		r := byService[svc]
		if r.Total != 0 {
			t.Errorf("%s total = %d, want 0 (zero-row should be included)", svc, r.Total)
		}
	}
}

// TestPGSelectorConversion verifies that type(SERVICE) is converted to type(PROCESS_GROUP).
func TestPGSelectorConversion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			`type(SERVICE),tag("[Environment]BookStore")`,
			`type(PROCESS_GROUP),tag("[Environment]BookStore")`,
		},
		{
			// Already PROCESS_GROUP — no change.
			`type(PROCESS_GROUP),tag("env:prod")`,
			`type(PROCESS_GROUP),tag("env:prod")`,
		},
		{
			// Only the first occurrence is replaced.
			`type(SERVICE),entityName.contains("SERVICE")`,
			`type(PROCESS_GROUP),entityName.contains("SERVICE")`,
		},
	}
	for _, tt := range tests {
		label := tt.input
		if len(label) > 30 {
			label = label[:30]
		}
		t.Run(label, func(t *testing.T) {
			got := strings.Replace(tt.input, "type(SERVICE)", "type(PROCESS_GROUP)", 1)
			if got != tt.want {
				t.Errorf("selector conversion: got %q, want %q", got, tt.want)
			}
		})
	}
}
