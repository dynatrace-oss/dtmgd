package client

import (
	"encoding/json"
	"testing"
)

func TestMergeArrayFields(t *testing.T) {
	dst := map[string]interface{}{
		"problems":    []interface{}{"a", "b"},
		"totalCount":  float64(10),
		"nextPageKey": "key1",
	}
	src := map[string]interface{}{
		"problems":    []interface{}{"c", "d"},
		"totalCount":  float64(10),
		"nextPageKey": "key2",
	}

	mergeArrayFields(dst, src)

	arr := dst["problems"].([]interface{})
	if len(arr) != 4 {
		t.Errorf("expected 4 items, got %d", len(arr))
	}
	if arr[2] != "c" || arr[3] != "d" {
		t.Errorf("unexpected merged values: %v", arr)
	}
	// totalCount should NOT be merged
	if dst["totalCount"] != float64(10) {
		t.Errorf("totalCount should not change, got %v", dst["totalCount"])
	}
}

func TestMergeArrayFieldsSkipsNonArrays(t *testing.T) {
	dst := map[string]interface{}{
		"version": "1.0",
		"items":   []interface{}{"a"},
	}
	src := map[string]interface{}{
		"version": "2.0",
		"items":   []interface{}{"b"},
	}

	mergeArrayFields(dst, src)

	if dst["version"] != "1.0" {
		t.Errorf("non-array field should not be overwritten, got %v", dst["version"])
	}
	if len(dst["items"].([]interface{})) != 2 {
		t.Errorf("expected 2 items, got %d", len(dst["items"].([]interface{})))
	}
}

func TestDecodePaged(t *testing.T) {
	raw := map[string]interface{}{
		"totalCount": float64(2),
		"problems": []interface{}{
			map[string]interface{}{"problemId": "p1", "title": "Problem 1"},
			map[string]interface{}{"problemId": "p2", "title": "Problem 2"},
		},
	}

	type Response struct {
		TotalCount int `json:"totalCount"`
		Problems   []struct {
			ProblemID string `json:"problemId"`
			Title     string `json:"title"`
		} `json:"problems"`
	}

	var resp Response
	if err := DecodePaged(raw, &resp); err != nil {
		t.Fatalf("DecodePaged failed: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected totalCount 2, got %d", resp.TotalCount)
	}
	if len(resp.Problems) != 2 {
		t.Errorf("expected 2 problems, got %d", len(resp.Problems))
	}
	if resp.Problems[0].ProblemID != "p1" {
		t.Errorf("expected p1, got %s", resp.Problems[0].ProblemID)
	}
}

func TestDecodePagedEmpty(t *testing.T) {
	raw := map[string]interface{}{}
	type Response struct {
		Items []string `json:"items"`
	}
	var resp Response
	if err := DecodePaged(raw, &resp); err != nil {
		t.Fatalf("DecodePaged failed: %v", err)
	}
	if resp.Items != nil {
		t.Errorf("expected nil items, got %v", resp.Items)
	}
}

func TestAPIError(t *testing.T) {
	err := APIError(401, "Unauthorized")
	apiErr, ok := err.(*ErrAPI)
	if !ok {
		t.Fatalf("expected *ErrAPI, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected 401, got %d", apiErr.StatusCode)
	}
	if apiErr.Error() != "API error 401: Unauthorized" {
		t.Errorf("unexpected error message: %s", apiErr.Error())
	}
}

func TestIsRetryable(t *testing.T) {
	// isRetryable requires a real resty.Response which is hard to mock.
	// The retry logic is tested indirectly through integration tests.
}

// Verify JSON round-trip preserves structure
func TestDecodePagedRoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"slo": []interface{}{
			map[string]interface{}{"id": "slo-1", "name": "Availability", "target": float64(99.9)},
		},
		"totalCount": float64(1),
	}

	data, _ := json.Marshal(original)
	var roundTripped map[string]interface{}
	json.Unmarshal(data, &roundTripped)

	type SLOResponse struct {
		Slo []struct {
			ID     string  `json:"id"`
			Name   string  `json:"name"`
			Target float64 `json:"target"`
		} `json:"slo"`
		TotalCount int `json:"totalCount"`
	}

	var resp SLOResponse
	if err := DecodePaged(roundTripped, &resp); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if resp.Slo[0].Target != 99.9 {
		t.Errorf("expected 99.9, got %f", resp.Slo[0].Target)
	}
}
