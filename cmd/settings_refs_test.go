package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// schemaCatalog is the schema list and one definition route per schema, at
// the schemas path prefix (without the method).
func schemaCatalog(prefix string, defs map[string]string) map[string]route {
	routes := map[string]route{}
	var items []string
	for id, def := range defs {
		items = append(items, fmt.Sprintf(`{"schemaId":%q}`, id))
		routes["GET "+prefix+"/"+id] = route{Body: def}
	}
	routes["GET "+prefix] = route{Body: fmt.Sprintf(`{"items":[%s],"totalCount":%d}`, strings.Join(items, ","), len(items))}
	return routes
}

func TestReferencingSchemas(t *testing.T) {
	defs := map[string]json.RawMessage{
		"builtin:problem.notifications": json.RawMessage(`{"properties":{"alertingProfile":{"type":"setting","referencedType":"builtin:alerting.profile"}}}`),
		"builtin:x.nested":              json.RawMessage(`{"types":{"F":{"properties":{"mz":{"type":"list","items":{"type":"setting","referencedType":"builtin:alerting.profile"}}}}}}`),
		"builtin:other":                 json.RawMessage(`{"properties":{"a":{"type":"setting","referencedType":"builtin:management-zones"}}}`),
		"builtin:plain":                 json.RawMessage(`{"properties":{"a":{"type":"text","referencedType":"builtin:alerting.profile"}}}`),
	}
	got := strings.Join(referencingSchemas(defs, "builtin:alerting.profile"), ",")
	if got != "builtin:problem.notifications,builtin:x.nested" {
		t.Errorf("got %s", got)
	}
}
