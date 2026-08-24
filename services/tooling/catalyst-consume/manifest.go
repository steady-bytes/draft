package main

import (
	"fmt"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

// pluginName and pluginVersion identify this plugin's published catalog
// entry — see services/tooling/slack-notify/manifest.go's identical
// comment for why they're named constants rather than inlined.
const (
	pluginName    = "catalyst-consume"
	pluginVersion = "v1"
)

// buildManifest constructs the PublishRequest catalyst-consume sends to
// Garage's PluginCatalogService on startup.
func buildManifest() (*plugincatalogv1.PublishRequest, error) {
	configSchema, err := structpb.NewStruct(map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"event_type"},
		"properties": map[string]interface{}{
			"event_type": map[string]interface{}{
				"type":        "string",
				"description": "CloudEvent type to wait for, e.g. \"tooling.workflow.v1.RunFinished\".",
			},
			"timeout": map[string]interface{}{
				"type":        "string",
				"description": "Go duration string, e.g. \"10s\". Defaults to 10s if unset.",
			},
			"fields": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": map[string]interface{}{"type": "string"},
				"description":          "output_key -> dot-path into the matched event's decoded JSON payload, e.g. {\"run_id\": \"runId\"}. Each resolved value is copied to that key on the step's result.",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build config_schema: %w", err)
	}

	resultSchema, err := structpb.NewStruct(map[string]interface{}{
		"type":        "object",
		"description": "Every key requested via config.fields, plus _event (type/source/id/subject metadata) and payload (the full decoded event body).",
		"properties": map[string]interface{}{
			"_event": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type":    map[string]interface{}{"type": "string"},
					"source":  map[string]interface{}{"type": "string"},
					"id":      map[string]interface{}{"type": "string"},
					"subject": map[string]interface{}{"type": "string"},
				},
			},
			"payload": map[string]interface{}{"type": "object"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build result_schema: %w", err)
	}

	return &plugincatalogv1.PublishRequest{
		Name:         pluginName,
		Version:      pluginVersion,
		Description:  "Waits for the next Catalyst CloudEvent of a configured type and extracts named fields from its payload for later steps.",
		Maintainer:   "platform-team",
		Source:       "https://github.com/steady-bytes/draft/tree/main/services/tooling/catalyst-consume",
		ConfigSchema: configSchema,
		ResultSchema: resultSchema,
	}, nil
}
