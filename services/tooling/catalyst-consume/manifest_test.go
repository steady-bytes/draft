package main

import "testing"

func TestBuildManifest(t *testing.T) {
	m, err := buildManifest()
	if err != nil {
		t.Fatalf("buildManifest returned unexpected error: %v", err)
	}

	if m.GetName() != pluginName {
		t.Errorf("Name = %q, want %q", m.GetName(), pluginName)
	}
	if m.GetVersion() != pluginVersion {
		t.Errorf("Version = %q, want %q", m.GetVersion(), pluginVersion)
	}
	if m.GetDescription() == "" {
		t.Error("Description is empty")
	}
	if m.GetMaintainer() == "" {
		t.Error("Maintainer is empty")
	}
	if m.GetSource() == "" {
		t.Error("Source is empty")
	}

	configFields := m.GetConfigSchema().GetFields()
	if got := configFields["type"].GetStringValue(); got != "object" {
		t.Errorf("config_schema.type = %q, want %q", got, "object")
	}
	required := configFields["required"].GetListValue().GetValues()
	if len(required) != 1 || required[0].GetStringValue() != "event_type" {
		t.Errorf("config_schema.required = %v, want [\"event_type\"]", required)
	}

	properties := configFields["properties"].GetStructValue().GetFields()
	for _, key := range []string{"event_type", "timeout", "fields"} {
		if _, ok := properties[key]; !ok {
			t.Errorf("config_schema.properties.%s is missing", key)
		}
	}
}
