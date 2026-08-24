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
	if len(required) != 2 {
		t.Fatalf("config_schema.required has %d entries, want 2", len(required))
	}

	properties := configFields["properties"].GetStructValue().GetFields()
	if _, ok := properties["channel"]; !ok {
		t.Error("config_schema.properties.channel is missing")
	}
	if _, ok := properties["message"]; !ok {
		t.Error("config_schema.properties.message is missing")
	}

	resultProperties := m.GetResultSchema().GetFields()["properties"].GetStructValue().GetFields()
	if _, ok := resultProperties["message_ts"]; !ok {
		t.Error("result_schema.properties.message_ts is missing")
	}
}
