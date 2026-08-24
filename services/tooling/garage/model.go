package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	plugincatalogv1 "github.com/steady-bytes/draft/api/tooling/plugin_catalog/v1"
	pgbun "github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// pluginRow is the bun-mapped row for the `plugins` table.
//
// One row per published *version* of a plugin: publishing a new version
// creates a new row rather than updating a previous one, since published
// versions are immutable and only Retract removes one (see
// docs/website/content/docs/architecture/garage-plugin-repository.md, "The
// catalog"). The (name, version) pair is how a plugin is looked up, so it's
// enforced here as a composite unique constraint.
//
// The generated plugincatalogv1.Plugin type is deliberately not used
// directly as a bun model (the way examples/crud's Name is): two of its
// fields, config_schema and result_schema, are google.protobuf.Struct, and
// published_at is a google.protobuf.Timestamp — none of which bun maps onto
// a column the way a plain string does. This row type and the toProto/
// newPluginRow conversions below are the bridge between the two.
type pluginRow struct {
	bun.BaseModel `bun:"table:plugins,alias:p"`

	ID          string `bun:"id,pk,type:uuid"`
	Name        string `bun:"name,notnull,unique:plugins_name_version"`
	Version     string `bun:"version,notnull,unique:plugins_name_version"`
	Description string `bun:"description"`
	Maintainer  string `bun:"maintainer"`
	Source      string `bun:"source"`

	// ConfigSchema/ResultSchema hold the plugin's JSON Schema documents
	// (config_schema/result_schema on the proto Plugin, both
	// google.protobuf.Struct) as jsonb. json.RawMessage round-trips through
	// bun/pgdriver as jsonb without an intermediate Go struct type.
	ConfigSchema json.RawMessage `bun:"config_schema,type:jsonb"`
	ResultSchema json.RawMessage `bun:"result_schema,type:jsonb"`

	PublishedAt time.Time `bun:"published_at,notnull"`
}

// newPluginRow converts a plugincatalogv1.Plugin into its bun row
// representation, generating a fresh row ID and defaulting PublishedAt to
// now if the proto didn't carry one (e.g. a not-yet-published manifest).
func newPluginRow(p *plugincatalogv1.Plugin) (*pluginRow, error) {
	configSchema, err := structToJSON(p.GetConfigSchema())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config_schema: %w", err)
	}
	resultSchema, err := structToJSON(p.GetResultSchema())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result_schema: %w", err)
	}

	publishedAt := time.Now().UTC()
	if p.GetPublishedAt() != nil {
		publishedAt = p.GetPublishedAt().AsTime()
	}

	return &pluginRow{
		ID:           uuid.New().String(),
		Name:         p.GetName(),
		Version:      p.GetVersion(),
		Description:  p.GetDescription(),
		Maintainer:   p.GetMaintainer(),
		Source:       p.GetSource(),
		ConfigSchema: configSchema,
		ResultSchema: resultSchema,
		PublishedAt:  publishedAt,
	}, nil
}

// toProto converts a pluginRow back into the generated Plugin type served
// over PluginCatalogService.
func (r *pluginRow) toProto() (*plugincatalogv1.Plugin, error) {
	configSchema, err := jsonToStruct(r.ConfigSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config_schema: %w", err)
	}
	resultSchema, err := jsonToStruct(r.ResultSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal result_schema: %w", err)
	}

	return &plugincatalogv1.Plugin{
		Name:         r.Name,
		Version:      r.Version,
		Description:  r.Description,
		Maintainer:   r.Maintainer,
		Source:       r.Source,
		ConfigSchema: configSchema,
		ResultSchema: resultSchema,
		PublishedAt:  timestamppb.New(r.PublishedAt),
	}, nil
}

// structToJSON marshals a google.protobuf.Struct to raw JSON for storage in
// a jsonb column. A nil Struct (config_schema/result_schema are optional on
// a manifest) marshals to JSON null rather than erroring.
func structToJSON(s *structpb.Struct) (json.RawMessage, error) {
	if s == nil {
		return json.RawMessage("null"), nil
	}
	b, err := protojson.Marshal(s)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// jsonToStruct unmarshals a jsonb column's raw JSON back into a
// google.protobuf.Struct. Empty/null input yields a nil Struct.
func jsonToStruct(raw json.RawMessage) (*structpb.Struct, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	s := &structpb.Struct{}
	if err := protojson.Unmarshal(raw, s); err != nil {
		return nil, err
	}
	return s, nil
}

// createSchema creates the `plugins` table if it doesn't already exist. This
// is the gap Phase 1 deliberately left open (see main.go's doc comment and
// docs/website/content/docs/architecture/garage-plugin-repository.md's Phase
// 1 note): pluginRow above existed with no table behind it until Phase 2's
// RPCs needed real persistence to implement against. Mirrors
// services/tooling/bench/model.go's createSchema, including the same
// log-don't-fail behavior: called from main via WithRunner, so a missing
// local Postgres doesn't block the rest of Garage's startup.
func createSchema(ctx context.Context, db pgbun.Repository) error {
	if _, err := db.Client().NewCreateTable().Model((*pluginRow)(nil)).IfNotExists().Exec(ctx); err != nil {
		return fmt.Errorf("failed to create table for %T: %w", (*pluginRow)(nil), err)
	}
	return nil
}
