// This file fills the gap Phase 4/5's brief calls out explicitly: nothing before
// this phase populated the `workflows` table. Phase 2's loader (loader.go's
// LoadWorkflowFile/ParseWorkflow) parses and validates a single YAML file; this is
// that, applied to every file in a configured directory at startup — plus, as of
// Phase 10, seed-once semantics: a file only ever creates a workflow the first
// time its name is seen. See the doc's "Authoring workflows without a file"
// section: workflows_dir is a seed source, not something that keeps re-syncing
// its contents into the database on every restart — once a workflow exists (from
// a file or from the UI/RPCs), it's DB-owned from then on, and editing the file
// on disk does nothing further. This is what makes it safe for the UI's
// Create/Update/Delete (workflow_write.go, rpc.go, ui.go) to actually own a
// workflow's definition without a later restart silently reverting their edits.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/steady-bytes/draft/pkg/chassis"
)

// defaultWorkflowsDir is used when the "bench.workflows_dir" config key is unset.
const defaultWorkflowsDir = "./workflows"

// loadWorkflowsDir reads every *.yaml/*.yml file directly inside dir
// (non-recursive), parses and validates each one via loader.go, and seeds it
// into Postgres — but only if no workflow with that name already exists.
// Returns how many workflows were newly seeded this run.
//
// A missing workflows_dir is not fatal — it just means nothing seeds this run,
// which is fine for a deployment that authors every workflow through the UI
// instead. A malformed workflow file inside an existing workflows_dir is fatal
// to startup, though: silently skipping it would leave an author's mistake
// undetected until the first time something tried to trigger it, the same
// "reject at load time, not run time" discipline loader.go already applies
// within a single file. Note this still parses+validates a file whose name
// already exists in the DB (so a bad edit to an already-seeded file's YAML is
// still caught at startup) — it just doesn't persist the result, since that
// workflow is DB-owned now.
func loadWorkflowsDir(ctx context.Context, dir string, store *pgResultStore, logger chassis.Logger) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.WithField("workflows_dir", dir).Warn("workflows_dir does not exist; no workflows seeded")
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read workflows_dir %q: %w", dir, err)
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml":
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths) // deterministic load order

	existing, err := store.ListWorkflows(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list existing workflows: %w", err)
	}
	alreadySeeded := make(map[string]bool, len(existing))
	for _, w := range existing {
		alreadySeeded[w.GetName()] = true
	}

	seeded := 0
	for _, path := range paths {
		w, err := LoadWorkflowFile(path)
		if err != nil {
			return seeded, fmt.Errorf("failed to load workflow from %s: %w", path, err)
		}

		if alreadySeeded[w.GetName()] {
			logger.WithField("workflow", w.GetName()).WithField("path", path).
				Info("workflow already exists in the database; not re-seeding from file (edit it through the UI instead)")
			continue
		}

		if err := store.UpsertWorkflow(ctx, w); err != nil {
			return seeded, fmt.Errorf("failed to seed workflow %q from %s: %w", w.GetName(), path, err)
		}
		logger.WithField("workflow", w.GetName()).WithField("path", path).Info("seeded workflow from file")
		seeded++
	}
	return seeded, nil
}
