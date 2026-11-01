package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// secretPlaceholder is what a dry-run report shows in place of a secret.
const secretPlaceholder = "<secret>"

// maskedSecret matches the cluster's mask for a stored secret.
var maskedSecret = regexp.MustCompile(`^\*\*\*[0-9a-f]{16}\*\*\*$`)

// hasSecrets reports whether the schema holds a secret property anywhere.
func (s settingsSchemaShape) hasSecrets() bool {
	if propsHoldSecret(s.Properties) {
		return true
	}
	for _, t := range s.Types {
		if propsHoldSecret(t.Properties) {
			return true
		}
	}
	return false
}

// propsHoldSecret reports whether a property is a secret or a collection of
// secrets.
func propsHoldSecret(props map[string]schemaProperty) bool {
	for _, p := range props {
		if typeName(p.Type) == "secret" || (p.Items != nil && typeName(p.Items.Type) == "secret") {
			return true
		}
	}
	return false
}

// typeName is a plain type name, or "" for a $ref.
func typeName(raw json.RawMessage) string {
	var name string
	if json.Unmarshal(raw, &name) != nil {
		return ""
	}
	return name
}

// typeKey is the name of the type a $ref names, or "".
func typeKey(raw json.RawMessage) string {
	typ, enum := typeRef(raw)
	if enum != "" || typ == "—" || typeName(raw) != "" {
		return ""
	}
	return typ
}

// visitFunc sees one secret. It returns the value to store in its place and
// whether to store it at all: a read-only walk never stores, so it never
// writes into data other goroutines share.
type visitFunc func(path string, v interface{}) (interface{}, bool)

// maskSecrets returns a deep copy of value in which every non-nil secret is
// the placeholder.
func maskSecrets(s settingsSchemaShape, value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	cp, _ := deepCopy(value).(map[string]interface{})
	s.walkSecrets(s.Properties, cp, "", func(_ string, v interface{}) (interface{}, bool) {
		if v == nil {
			return nil, false
		}
		return secretPlaceholder, true
	})
	return cp
}

// maskedSecretPaths lists, sorted, the paths of secrets in value that hold
// the cluster's mask instead of a secret.
func maskedSecretPaths(s settingsSchemaShape, value map[string]interface{}) []string {
	var paths []string
	s.walkSecrets(s.Properties, value, "", func(path string, v interface{}) (interface{}, bool) {
		if str, ok := v.(string); ok && maskedSecret.MatchString(str) {
			paths = append(paths, path)
		}
		return nil, false
	})
	sort.Strings(paths)
	return paths
}

// deepCopy copies maps and lists; scalars are shared.
func deepCopy(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{}, len(t))
		for k, e := range t {
			m[k] = deepCopy(e)
		}
		return m
	case []interface{}:
		l := make([]interface{}, len(t))
		for i, e := range t {
			l[i] = deepCopy(e)
		}
		return l
	}
	return v
}

// walkSecrets visits every secret in m, described by props, and stores what
// visit returns in its place. A property missing from m is skipped.
func (s settingsSchemaShape) walkSecrets(props map[string]schemaProperty, m map[string]interface{}, prefix string, visit visitFunc) {
	for name, p := range props {
		_, ok := m[name]
		if !ok {
			continue
		}
		s.walkProperty(p, m, name, prefix+name, visit)
	}
}

// walkProperty visits the secrets in m[name], whose property is p.
func (s settingsSchemaShape) walkProperty(p schemaProperty, m map[string]interface{}, name, path string, visit visitFunc) {
	v := m[name]
	if typeName(p.Type) == "secret" {
		if nv, replace := visit(path, v); replace {
			m[name] = nv
		}
		return
	}
	if key := typeKey(p.Type); key != "" {
		if obj, ok := v.(map[string]interface{}); ok {
			s.walkSecrets(s.Types[key].Properties, obj, path+".", visit)
		}
		return
	}
	if list, ok := v.([]interface{}); ok && p.Items != nil {
		s.walkItems(p.Items.Type, list, path, visit)
	}
}

// walkItems visits the secrets in a list whose items have type raw.
func (s settingsSchemaShape) walkItems(raw json.RawMessage, list []interface{}, path string, visit visitFunc) {
	secret, key := typeName(raw) == "secret", typeKey(raw)
	for i, item := range list {
		p := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case secret:
			if nv, replace := visit(p, item); replace {
				list[i] = nv
			}
		case key != "":
			if obj, ok := item.(map[string]interface{}); ok {
				s.walkSecrets(s.Types[key].Properties, obj, p+".", visit)
			}
		}
	}
}

// guardSecrets keeps secrets out of reports and masks out of creates, in
// place. It refuses a create holding the cluster's mask: the POST would
// store the mask as the secret. For a dry run it replaces every secret in
// the reported values by a placeholder. Schemas are read once, and only when
// a write needs one.
func guardSecrets(c *client.Client, ws []plannedWrite, dryRun bool) error {
	shapes := map[string]settingsSchemaShape{}
	for i := range ws {
		w := &ws[i]
		if w.Exec != nil {
			// Not a settings object: it carries no secrets.
			w.ReportValue = w.Value
			continue
		}
		if w.SchemaID == "" {
			continue
		}
		if !needsSchema(*w, dryRun) {
			continue
		}
		shape, err := cachedShape(c, shapes, w.SchemaID)
		if err != nil {
			return err
		}
		if err := guardWrite(shape, w, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// needsSchema: a create is checked for masks, a dry run's values are masked.
func needsSchema(w plannedWrite, dryRun bool) bool {
	if w.Op == config.OpCreate {
		return true
	}
	return dryRun && (w.Value != nil || w.Current != nil)
}

// cachedShape reads a schema's definition once.
func cachedShape(c *client.Client, shapes map[string]settingsSchemaShape, schemaID string) (settingsSchemaShape, error) {
	if shape, ok := shapes[schemaID]; ok {
		return shape, nil
	}
	shape, err := schemaShape(c, schemaID)
	if err != nil {
		return shape, err
	}
	shapes[schemaID] = shape
	return shape, nil
}

// guardWrite refuses a create holding a mask and masks a dry run's values.
func guardWrite(shape settingsSchemaShape, w *plannedWrite, dryRun bool) error {
	if w.Op == config.OpCreate {
		if paths := maskedSecretPaths(shape, w.Value); len(paths) > 0 {
			return withHint(&UsageError{Err: fmt.Errorf("creating a %s object at %s: %s hold(s) a masked secret (***…***), not the secret itself", w.SchemaID, w.Scope, strings.Join(paths, ", "))},
				"Exports mask secrets. Put the real value in the file; dtmgd never prints it.")
		}
	}
	if dryRun && !shape.hasSecrets() {
		w.ReportValue = w.Value
		return nil
	}
	if dryRun {
		w.Current = maskSecrets(shape, w.Current)
		w.ReportValue = maskSecrets(shape, w.Value)
	}
	return nil
}
