package client

import (
	"encoding/json"
	"fmt"
)

// GetV2Paged performs a paginated GET, following nextPageKey until all pages are fetched.
// It merges array fields across pages and returns the combined result.
// maxPages limits the number of pages fetched (0 = unlimited).
func (c *Client) GetV2Paged(path string, params map[string]string, maxPages int) (map[string]interface{}, error) {
	if params == nil {
		params = map[string]string{}
	}

	var merged map[string]interface{}
	page := 0

	for {
		page++
		if maxPages > 0 && page > maxPages {
			break
		}

		raw, err := c.getV2Raw(path, params)
		if err != nil {
			return nil, err
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		if merged == nil {
			merged = parsed
		} else {
			mergeArrayFields(merged, parsed)
		}

		npk, _ := parsed["nextPageKey"].(string)
		if npk == "" {
			break
		}

		// For subsequent pages, only send nextPageKey (API requirement)
		params = map[string]string{"nextPageKey": npk}
	}

	if merged == nil {
		merged = map[string]interface{}{}
	}
	// Remove nextPageKey from final merged result since we've consumed all pages
	delete(merged, "nextPageKey")
	return merged, nil
}

// getV2Raw performs a single GET and returns the raw response body.
func (c *Client) getV2Raw(path string, params map[string]string) ([]byte, error) {
	req := c.http.R()
	for k, v := range params {
		req.SetQueryParam(k, v)
	}
	resp, err := req.Get("/v2" + path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return nil, APIError(resp.StatusCode(), resp.String())
	}
	return resp.Body(), nil
}

// mergeArrayFields appends array values from src into dst for matching keys.
func mergeArrayFields(dst, src map[string]interface{}) {
	for k, sv := range src {
		switch k {
		case "nextPageKey", "totalCount", "pageSize":
			continue
		}
		srcArr, srcOk := sv.([]interface{})
		if !srcOk {
			continue
		}
		dstArr, dstOk := dst[k].([]interface{})
		if !dstOk {
			continue
		}
		dst[k] = append(dstArr, srcArr...)
	}
}

// DecodePaged re-marshals a map into a typed struct.
func DecodePaged(raw map[string]interface{}, result interface{}) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}
