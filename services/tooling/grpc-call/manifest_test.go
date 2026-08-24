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
	if len(required) != 3 {
		t.Fatalf("config_schema.required = %v, want 3 entries", required)
	}
	for i, want := range []string{"address", "service", "method"} {
		if required[i].GetStringValue() != want {
			t.Errorf("config_schema.required[%d] = %q, want %q", i, required[i].GetStringValue(), want)
		}
	}

	properties := configFields["properties"].GetStructValue().GetFields()
	for _, key := range []string{"address", "service", "method", "request", "tls", "metadata", "timeout", "expect"} {
		if _, ok := properties[key]; !ok {
			t.Errorf("config_schema.properties.%s is missing", key)
		}
	}
}
