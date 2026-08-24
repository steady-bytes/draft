// This file implements Phase 10 (docs/website/content/docs/architecture/
// bench-workflow-engine.md's "Authoring workflows without a file"):
// createWorkflow/updateWorkflow, the shared business logic behind both
// rpc.go's CreateWorkflow/UpdateWorkflow RPCs and ui.go's New/Edit workflow
// form handlers, so the two entry points can't drift on what "create" and
// "update" actually mean. Both reuse loader.go's ParseWorkflow verbatim for
// parsing and validation -- no second workflow-authoring code path.
package main

import (
	"context"
	"errors"
	"fmt"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
)

// ErrWorkflowAlreadyExists is createWorkflow's failure when name is already
// taken -- Create is a real create, not the store's own unconditional
// upsert (see the design doc's "Create vs. Update, deliberately not one
// upsert").
var ErrWorkflowAlreadyExists = errors.New("workflow already exists")

// ErrWorkflowNameMismatch is updateWorkflow's failure when yaml's own
// metadata.name doesn't match the workflow being updated -- a rename isn't
// supported (see the design doc's Open questions).
var ErrWorkflowNameMismatch = errors.New("yaml metadata.name does not match the workflow being updated")

// workflowWriteStore is the persistence surface createWorkflow/
// updateWorkflow need -- satisfied by *pgResultStore, narrowed so tests can
// substitute a fake without a real Postgres instance.
type workflowWriteStore interface {
	GetWorkflow(ctx context.Context, name string) (*workflowv1.Workflow, error)
	UpsertWorkflow(ctx context.Context, w *workflowv1.Workflow) error
}

// createWorkflow parses+validates yaml and persists it as a new workflow.
// Fails ErrWorkflowAlreadyExists if name is already taken, so a "New
// workflow" submission that collides is steered toward Update instead of
// silently overwriting something.
func createWorkflow(ctx context.Context, store workflowWriteStore, yamlDoc string) (*workflowv1.Workflow, error) {
	w, err := ParseWorkflow([]byte(yamlDoc))
	if err != nil {
		return nil, err
	}

	if _, err := store.GetWorkflow(ctx, w.GetName()); err == nil {
		return nil, fmt.Errorf("%w: %q", ErrWorkflowAlreadyExists, w.GetName())
	} else if !errors.Is(err, ErrWorkflowNotFound) {
		return nil, err
	}

	if err := store.UpsertWorkflow(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// updateWorkflow parses+validates yaml and replaces name's definition.
// Fails ErrWorkflowNotFound if name doesn't already exist, and
// ErrWorkflowNameMismatch if yaml's own metadata.name differs from name.
func updateWorkflow(ctx context.Context, store workflowWriteStore, name, yamlDoc string) (*workflowv1.Workflow, error) {
	w, err := ParseWorkflow([]byte(yamlDoc))
	if err != nil {
		return nil, err
	}
	if w.GetName() != name {
		return nil, fmt.Errorf("%w: %q was submitted to update %q", ErrWorkflowNameMismatch, w.GetName(), name)
	}

	if _, err := store.GetWorkflow(ctx, name); err != nil {
		return nil, err // ErrWorkflowNotFound propagates as-is
	}

	if err := store.UpsertWorkflow(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}
