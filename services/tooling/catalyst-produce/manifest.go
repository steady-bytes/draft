package main

import (
	"fmt"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

// pluginName and pluginVersion identify this plugin's published catalog
// entry — see services/tooling/catalyst-consume/manifest.go's identical
// comment for why they're named constants rather than inlined.
const (
	pluginName    = "catalyst-produce"
	pluginVersion = "v1"
)

// buildManifest constructs the PublishRequest catalyst-produce sends to
// Garage's PluginCatalogService on startup.
func buildManifest() (*plugincatalogv1.PublishRequest, error) {
	configSchema, err := structpb.NewStruct(map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"event_type"},
		"properties": map[string]interface{}{
			"event_type": map[string]interface{}{
				"type":        "string",
				"description": "CloudEvent type to publish, e.g. \"myapp.v1.OrderPlaced\".",
			},
			"source": map[string]interface{}{
				"type":        "string",
				"description": "CloudEvent source URI-reference. Defaults to \"/plugins/catalyst-produce\" if unset.",
			},
			"subject": map[string]interface{}{
				"type":        "string",
				"description": "Optional CloudEvent subject attribute.",
			},
			"data": map[string]interface{}{
				"type":        "object",
				"description": "Arbitrary JSON payload, sent as the event's text data.",
			},
			"delay": map[string]interface{}{
				"type":        "string",
				"description": "Go duration string, e.g. \"500ms\". Waits this long before publishing — useful for giving a concurrent catalyst-consume@v1 step in the same run time to open its stream first, since Catalyst does not replay missed events. Defaults to no delay.",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build config_schema: %w", err)
	}

	resultSchema, err := structpb.NewStruct(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"event_id":     map[string]interface{}{"type": "string"},
			"type":         map[string]interface{}{"type": "string"},
			"source":       map[string]interface{}{"type": "string"},
			"published_at": map[string]interface{}{"type": "string"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build result_schema: %w", err)
	}

	return &plugincatalogv1.PublishRequest{
		Name:         pluginName,
		Version:      pluginVersion,
		Description:  "Publishes a CloudEvent to Catalyst with a configured type and JSON payload.",
		Maintainer:   "platform-team",
		Source:       "https://github.com/steady-bytes/draft/tree/main/services/tooling/catalyst-produce",
		ConfigSchema: configSchema,
		ResultSchema: resultSchema,
	}, nil
}
