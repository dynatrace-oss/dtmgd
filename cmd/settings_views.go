package cmd

// settingsObject is a stored Settings 2.0 object as get settings asks for it
// (settingsObjectFields), or an effective value (which has origin instead of
// objectId, scope and updateToken).
type settingsObject struct {
	ObjectID      string                 `json:"objectId"`
	SchemaID      string                 `json:"schemaId"`
	Scope         string                 `json:"scope"`
	ExternalID    string                 `json:"externalId"`
	Summary       string                 `json:"summary"`
	UpdateToken   string                 `json:"updateToken"`
	SchemaVersion string                 `json:"schemaVersion"`
	Origin        string                 `json:"origin"`
	Value         map[string]interface{} `json:"value"`
}

// genericSettingsRow is the table row for objects of any one schema.
type genericSettingsRow struct {
	ObjectID string `table:"OBJECT-ID"`
	Scope    string `table:"SCOPE"`
	Summary  string `table:"SUMMARY,max=80"`
}

// genericMultiSchemaRow is the table row for objects of several schemas.
type genericMultiSchemaRow struct {
	Schema   string `table:"SCHEMA"`
	ObjectID string `table:"OBJECT-ID"`
	Scope    string `table:"SCOPE"`
	Summary  string `table:"SUMMARY,max=80"`
}

// settingsView turns get settings' results into table rows. It picks the
// table by the schema of the objects it gets: a readable view for the update
// schemas, a generic one for any other schema or a mix of schemas.
type settingsView struct{}

// rows is get settings' fetchSpec.Rows. body is a list (items) or one object.
func (v settingsView) rows(body interface{}) (interface{}, int, error) {
	m, _ := body.(map[string]interface{})
	var objs []settingsObject
	if err := decodeItems(m, &objs); err != nil {
		return nil, 0, err
	}
	if _, single := singleSchema(objs); !single {
		return genericMultiSchemaRows(objs)
	}
	return genericRows(objs)
}

// singleSchema returns the schema all objs belong to, and whether there is
// only one (true for no objects).
func singleSchema(objs []settingsObject) (string, bool) {
	if len(objs) == 0 {
		return "", true
	}
	for _, o := range objs[1:] {
		if o.SchemaID != objs[0].SchemaID {
			return "", false
		}
	}
	return objs[0].SchemaID, true
}

func genericRows(objs []settingsObject) (interface{}, int, error) {
	rows := make([]genericSettingsRow, 0, len(objs))
	for _, o := range objs {
		rows = append(rows, genericSettingsRow{ObjectID: o.ObjectID, Scope: o.Scope, Summary: o.Summary})
	}
	return rows, len(rows), nil
}

func genericMultiSchemaRows(objs []settingsObject) (interface{}, int, error) {
	rows := make([]genericMultiSchemaRow, 0, len(objs))
	for _, o := range objs {
		rows = append(rows, genericMultiSchemaRow{Schema: o.SchemaID, ObjectID: o.ObjectID, Scope: o.Scope, Summary: o.Summary})
	}
	return rows, len(rows), nil
}
