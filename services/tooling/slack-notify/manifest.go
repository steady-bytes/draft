package main

import (
	"fmt"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

// pluginName and pluginVersion identify this plugin's published catalog
// entry. They're also this plugin's own identity for the (name, version)
// pair Publish/Retract key off of, so they're kept together as named
// constants rather than inlined at each call site.
const (
	pluginName    = "slack-notify"
	pluginVersion = "v2"
)

// buildManifest constructs the PublishRequest slack-notify sends to Garage's
// PluginCatalogService on startup, matching the example manifest in
// docs/website/content/docs/architecture/garage-plugin-repository.md's "The
// catalog" section exactly: the same name, version, description, maintainer,
// source, config_schema, and result_schema.
func buildManifest() (*plugincatalogv1.PublishRequest, error) {
	configSchema, err := structpb.NewStruct(map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"channel", "message"},
		"properties": map[string]interface{}{
			"channel": map[string]interface{}{
				"type":    "string",
				"pattern": "^#",
			},
			"message": map[string]interface{}{
				"type": "string",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build config_schema: %w", err)
	}

	resultSchema, err := structpb.NewStruct(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message_ts": map[string]interface{}{
				"type": "string",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build result_schema: %w", err)
	}

	return &plugincatalogv1.PublishRequest{
		Name:         pluginName,
		Version:      pluginVersion,
		Description:  "Posts a message to a Slack channel via an incoming webhook.",
		Maintainer:   "platform-team",
		Source:       "https://github.com/steady-bytes/draft-plugins/tree/main/slack-notify",
		ConfigSchema: configSchema,
		ResultSchema: resultSchema,
	}, nil
}
