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
	pluginName    = "grpc-call"
	pluginVersion = "v1"
)

// buildManifest constructs the PublishRequest grpc-call sends to Garage's
// PluginCatalogService on startup.
func buildManifest() (*plugincatalogv1.PublishRequest, error) {
	configSchema, err := structpb.NewStruct(map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"address", "service", "method"},
		"properties": map[string]interface{}{
			"address": map[string]interface{}{
				"type":        "string",
				"description": "host:port of any gRPC server that has server reflection enabled — not limited to Draft/Connect services (see bench://grpc-call@v1, Bench's own built-in executor, for that narrower case).",
			},
			"service": map[string]interface{}{
				"type":        "string",
				"description": "Fully qualified proto service name, e.g. \"helloworld.Greeter\".",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "Method name, e.g. \"SayHello\". Only unary methods are supported.",
			},
			"request": map[string]interface{}{
				"type":        "object",
				"description": "JSON payload matching the request message's fields (proto3 JSON mapping). Defaults to {}.",
			},
			"tls": map[string]interface{}{
				"type":        "boolean",
				"description": "Use TLS (system root CAs) instead of a plaintext connection. Defaults to false.",
			},
			"metadata": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": map[string]interface{}{"type": "string"},
				"description":          "Extra gRPC request metadata to attach.",
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
						"description": "Either the string \"OK\" (grpc code OK) or an exact grpc status code name, e.g. \"NotFound\".",
					},
					"response": map[string]interface{}{
						"type":        "object",
						"description": "Recursively mirrors the decoded response message's shape; each leaf is an assertion object (exists/equals/matches).",
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build config_schema: %w", err)
	}

	resultSchema, err := structpb.NewStruct(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"response": map[string]interface{}{"type": "object", "description": "The decoded response message."},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build result_schema: %w", err)
	}

	return &plugincatalogv1.PublishRequest{
		Name:         pluginName,
		Version:      pluginVersion,
		Description:  "Makes a unary call to any gRPC server that exposes server reflection, resolving the request/response shape at runtime — no generated stubs required.",
		Maintainer:   "platform-team",
		Source:       "https://github.com/steady-bytes/draft/tree/main/services/tooling/grpc-call",
		ConfigSchema: configSchema,
		ResultSchema: resultSchema,
	}, nil
}
