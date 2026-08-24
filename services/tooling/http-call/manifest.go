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
	pluginName    = "http-call"
	pluginVersion = "v1"
)

// buildManifest constructs the PublishRequest http-call sends to Garage's
// PluginCatalogService on startup.
func buildManifest() (*plugincatalogv1.PublishRequest, error) {
	assertionSchema := map[string]interface{}{
		"type":        "object",
		"description": "A leaf assertion: any combination of exists/equals/matches.",
		"properties": map[string]interface{}{
			"exists":  map[string]interface{}{"type": "boolean"},
			"equals":  map[string]interface{}{"description": "Any JSON value the field must deep-equal."},
			"matches": map[string]interface{}{"type": "string", "description": "A regular expression the field's string value must match."},
		},
	}

	configSchema, err := structpb.NewStruct(map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"method", "url"},
		"properties": map[string]interface{}{
			"method": map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "Absolute URL to request.",
			},
			"headers": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": map[string]interface{}{"type": "string"},
			},
			"body": map[string]interface{}{
				"description": "Sent as the request body. An object/array is JSON-encoded (Content-Type: application/json unless headers overrides it); a string is sent verbatim.",
			},
			"timeout": map[string]interface{}{
				"type":        "string",
				"description": "Go duration string, e.g. \"10s\". Defaults to 10s if unset.",
			},
			"expect": map[string]interface{}{
				"type":        "object",
				"description": "Optional response assertions, evaluated by this plugin itself — see the plugin's own doc note on why this isn't the workflow's top-level expect: block.",
				"properties": map[string]interface{}{
					"status": map[string]interface{}{
						"description": "Either the string \"OK\" (any 2xx) or an exact integer status code, e.g. 404.",
					},
					"body": map[string]interface{}{
						"type":        "object",
						"description": "Recursively mirrors the parsed JSON response body's shape; each leaf is an assertion object (see assertionSchema).",
					},
				},
			},
		},
		"$defs": map[string]interface{}{
			"assertion": assertionSchema,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build config_schema: %w", err)
	}

	resultSchema, err := structpb.NewStruct(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"status":  map[string]interface{}{"type": "integer", "description": "HTTP status code."},
			"headers": map[string]interface{}{"type": "object", "description": "Response headers, first value per name."},
			"body":    map[string]interface{}{"description": "Parsed JSON response body, or the raw string if it wasn't valid JSON."},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build result_schema: %w", err)
	}

	return &plugincatalogv1.PublishRequest{
		Name:         pluginName,
		Version:      pluginVersion,
		Description:  "Makes a plain HTTP request (any method, headers, body) and optionally asserts on the status/response.",
		Maintainer:   "platform-team",
		Source:       "https://github.com/steady-bytes/draft/tree/main/services/tooling/http-call",
		ConfigSchema: configSchema,
		ResultSchema: resultSchema,
	}, nil
}
