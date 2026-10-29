package client

import (
	"encoding/json"
	"fmt"
)

// Paging controls how far GetV2Paged follows nextPageKey.
type Paging struct {
	// MaxPages stops after this many pages (0 = no page limit).
	MaxPages int
	// Limit stops once ListKey holds at least this many items, and truncates
	// the list to exactly Limit (0 = no item limit).
	Limit int
	// ListKey names the response's array field, e.g. "problems". Limit needs
	// it to count items.
	ListKey string
}

// GetV2Paged performs a paginated GET, following nextPageKey and merging the
// array fields of every page into the first page's object.
//
// It stops at the last page, at p.MaxPages, or as soon as p.Limit items are in
// hand, so a small --limit costs one or two requests rather than every page.
// When it stops with pages remaining, the result keeps the last nextPageKey:
// that is how callers, and JSON consumers, can tell the result is truncated.
// totalCount stays as the first page reported it.
func (c *Client) GetV2Paged(path string, params map[string]string, p Paging) (map[string]interface{}, error) {
	if params == nil {
		params = map[string]string{}
	}

	var merged map[string]interface{}
	npk := ""
	for page := 1; ; page++ {
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

		npk, _ = parsed["nextPageKey"].(string)
		if npk == "" || (p.MaxPages > 0 && page >= p.MaxPages) || p.limitReached(merged) {
			break
		}

		// For subsequent pages, only send nextPageKey (API requirement)
		params = map[string]string{"nextPageKey": npk}
	}

	if merged == nil {
		merged = map[string]interface{}{}
	}
	if npk == "" {
		delete(merged, "nextPageKey")
	} else {
		merged["nextPageKey"] = npk
	}
	p.truncate(merged)
	return merged, nil
}

func (p Paging) list(m map[string]interface{}) []interface{} {
	if p.ListKey == "" {
		return nil
	}
	l, _ := m[p.ListKey].([]interface{})
	return l
}

func (p Paging) limitReached(m map[string]interface{}) bool {
	return p.Limit > 0 && len(p.list(m)) >= p.Limit
}

func (p Paging) truncate(m map[string]interface{}) {
	if l := p.list(m); p.Limit > 0 && len(l) > p.Limit {
		m[p.ListKey] = l[:p.Limit]
	}
}

// getV2Raw performs a single GET and returns the raw response body.
func (c *Client) getV2Raw(path string, params map[string]string) ([]byte, error) {
	req := c.http.R()
	for k, v := range params {
		req.SetQueryParam(k, v)
	}
	resp, err := req.Get("/v2" + path)
	if err != nil {
		return nil, c.requestError("request", err)
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
