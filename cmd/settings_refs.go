package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// The delete check works for every schema: a schema declares a reference as
// a property of type "setting" with a referencedType, at any depth of its
// types and list items. Deleting an object refuses while an object of a
// referencing schema holds its ID.

const (
	// definitionWorkers is how many schema definitions are read at a time.
	definitionWorkers = 8
	// maxListedReferences is how many referencing objects a refusal names.
	maxListedReferences = 10
	hintReadSchemas     = "Reading settings schemas needs settings.read."
)

// schemaDefinitions reads every schema's definition. A definition that
// cannot be found (404) is skipped with a warning; a 403 asks for
// settings.read; any other failure is returned.
func schemaDefinitions(c *client.Client) (defs map[string]json.RawMessage, warnings []string, err error) {
	var list struct {
		Items []struct {
			SchemaID string `json:"schemaId"`
		} `json:"items"`
	}
	if err := settingsGet(c, settingsSchemasPath, nil, &list); err != nil {
		return nil, nil, readSchemasError(err)
	}
	defs = make(map[string]json.RawMessage, len(list.Items))
	var mu sync.Mutex
	var wg sync.WaitGroup
	ids := make(chan string)
	for i := 0; i < definitionWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range ids {
				var def json.RawMessage
				e := settingsGet(c, settingsSchemasPath+"/"+url.PathEscape(id), nil, &def)
				mu.Lock()
				switch {
				case e == nil:
					defs[id] = def
				case isNotFound(e):
					warnings = append(warnings, fmt.Sprintf("schema %s could not be read (404); the delete check skipped it", id))
				case err == nil:
					err = readSchemasError(e)
				}
				mu.Unlock()
			}
		}()
	}
	for _, it := range list.Items {
		ids <- it.SchemaID
	}
	close(ids)
	wg.Wait()
	sort.Strings(warnings)
	if err != nil {
		return nil, nil, err
	}
	return defs, warnings, nil
}

// isNotFound reports whether err is an HTTP 404.
func isNotFound(err error) bool {
	var apiErr *client.ErrAPI
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

// readSchemasError adds the settings.read hint to a 403.
func readSchemasError(err error) error {
	if isForbidden(err) {
		return withHint(err, hintReadSchemas)
	}
	return err
}

// referencingSchemas returns the sorted IDs of the schemas that declare a
// reference to target.
func referencingSchemas(defs map[string]json.RawMessage, target string) []string {
	var ids []string
	for id, raw := range defs {
		var v interface{}
		if json.Unmarshal(raw, &v) == nil && declaresReference(v, target) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// declaresReference reports whether some JSON object under v is a property
// of type "setting" whose referencedType is target.
func declaresReference(v interface{}, target string) bool {
	switch n := v.(type) {
	case map[string]interface{}:
		if n["type"] == "setting" && n["referencedType"] == target {
			return true
		}
		for _, child := range n {
			if declaresReference(child, target) {
				return true
			}
		}
	case []interface{}:
		for _, child := range n {
			if declaresReference(child, target) {
				return true
			}
		}
	}
	return false
}

// holdsString reports whether a string equal to s occurs anywhere in v.
func holdsString(v interface{}, s string) bool {
	switch n := v.(type) {
	case string:
		return n == s
	case map[string]interface{}:
		for _, child := range n {
			if holdsString(child, s) {
				return true
			}
		}
	case []interface{}:
		for _, child := range n {
			if holdsString(child, s) {
				return true
			}
		}
	}
	return false
}

// findReferences lists the objects of schemas, at every scope, whose value
// holds objectID.
func findReferences(c *client.Client, objectID string, schemas []string) ([]settingsObject, error) {
	body, err := settingsPaged(c, settingsObjectsPath, map[string]string{
		"schemaIds": strings.Join(schemas, ","),
		"fields":    "objectId,schemaId,scope,summary,value",
		"pageSize":  "500",
	}, client.Paging{ListKey: "items"})
	if err != nil {
		return nil, readSchemasError(err)
	}
	var objs []settingsObject
	if err := decodeItems(body, &objs); err != nil {
		return nil, err
	}
	var refs []settingsObject
	for _, o := range objs {
		if holdsString(o.Value, objectID) {
			refs = append(refs, o)
		}
	}
	return refs, nil
}

// checkUnreferenced refuses to delete cur while another settings object
// references it. The warnings name schemas the check could not read.
func checkUnreferenced(c *client.Client, cur settingsObject) (warnings []string, err error) {
	defs, warnings, err := schemaDefinitions(c)
	if err != nil {
		return nil, err
	}
	schemas := referencingSchemas(defs, cur.SchemaID)
	if len(schemas) == 0 {
		return warnings, nil
	}
	refs, err := findReferences(c, cur.ObjectID, schemas)
	if err != nil || len(refs) == 0 {
		return warnings, err
	}
	return nil, withHint(fmt.Errorf("%s object %s (%s) is still used by %d settings object(s): %s",
		cur.SchemaID, cur.ObjectID, cur.Summary, len(refs), listReferences(refs)),
		"Change or delete those settings first. Settings only: dashboards and other configuration outside Settings 2.0 are not checked.")
}

// listReferences names at most maxListedReferences of refs.
func listReferences(refs []settingsObject) string {
	var users []string
	for i, r := range refs {
		if i == maxListedReferences {
			break
		}
		users = append(users, fmt.Sprintf("%s at %s (%s, %s)", r.SchemaID, r.Scope, r.ObjectID, r.Summary))
	}
	out := strings.Join(users, ", ")
	if len(refs) > maxListedReferences {
		out += ", …"
	}
	return out
}
