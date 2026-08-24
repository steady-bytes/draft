// This file implements Phase 5: WorkflowService's RPCs (TriggerRun, GetRun,
// ListRuns, ListWorkflows) — see api/tooling/workflow/v1/service.proto — following
// the exact same handler shape services/tooling/garage/rpc.go and
// services/examples/crud/service/rpc.go already establish: a struct implementing
// both chassis.RPCRegistrar (so main.go can pass it to WithRPCHandler) and the
// generated *ServiceHandler interface, backed by store.go's persistence layer.
//
// TriggerRun takes the same "start async, return the run" path webhook.go's
// handler does (Scheduler.StartRun) — it's the UI/API entry point to the identical
// execution path a webhook triggers, just without a signature to check.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	workflowv1 "github.com/steady-bytes/draft/api/tooling/workflow/v1"
	workflowv1connect "github.com/steady-bytes/draft/api/tooling/workflow/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

type (
	Handler interface {
		chassis.RPCRegistrar
		workflowv1connect.WorkflowServiceHandler
	}
	handler struct {
		logger    chassis.Logger
		store     *pgResultStore
		scheduler workflowRunner
	}
)

func NewHandler(logger chassis.Logger, store *pgResultStore, scheduler workflowRunner) Handler {
	return &handler{
		logger:    logger,
		store:     store,
		scheduler: scheduler,
	}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := workflowv1connect.NewWorkflowServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

// TriggerRun starts a run of a workflow manually — the UI's "run now" path, and the
// same StartRun-then-return-immediately path the webhook handler uses.
func (h *handler) TriggerRun(ctx context.Context, req *connect.Request[workflowv1.TriggerRunRequest]) (*connect.Response[workflowv1.TriggerRunResponse], error) {
	name := strings.TrimSpace(req.Msg.GetWorkflowName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workflow_name is required"))
	}

	workflow, err := h.store.GetWorkflow(ctx, name)
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("workflow %q not found", name))
		}
		h.logger.WithError(err).WithField("workflow", name).Error("failed to load workflow")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load workflow"))
	}

	run, err := h.scheduler.StartRun(ctx, workflow)
	if err != nil {
		h.logger.WithError(err).WithField("workflow", name).Error("failed to start run")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to start run"))
	}

	return connect.NewResponse(&workflowv1.TriggerRunResponse{
		RunId:  run.GetRunId(),
		Status: run.GetStatus(),
	}), nil
}

// GetRun returns the current status of one run, polled by run_id — the RPC
// equivalent of the doc's `GET /runs/{id}`.
func (h *handler) GetRun(ctx context.Context, req *connect.Request[workflowv1.GetRunRequest]) (*connect.Response[workflowv1.Run], error) {
	runID := strings.TrimSpace(req.Msg.GetRunId())
	if runID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("run_id is required"))
	}

	run, err := h.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("run %q not found", runID))
		}
		h.logger.WithError(err).WithField("run_id", runID).Error("failed to get run")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get run"))
	}

	return connect.NewResponse(run), nil
}

// ListRuns returns run history, optionally filtered to one workflow.
func (h *handler) ListRuns(ctx context.Context, req *connect.Request[workflowv1.ListRunsRequest]) (*connect.Response[workflowv1.ListRunsResponse], error) {
	runs, nextPageToken, err := h.store.ListRuns(ctx, req.Msg.GetWorkflowName(), req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if err != nil {
		if errors.Is(err, ErrInvalidRunPageToken) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		h.logger.WithError(err).Error("failed to list runs")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list runs"))
	}

	return connect.NewResponse(&workflowv1.ListRunsResponse{
		Runs:          runs,
		NextPageToken: nextPageToken,
	}), nil
}

// ListWorkflows returns every workflow loaded from workflows_dir at startup.
func (h *handler) ListWorkflows(ctx context.Context, req *connect.Request[workflowv1.ListWorkflowsRequest]) (*connect.Response[workflowv1.ListWorkflowsResponse], error) {
	workflows, err := h.store.ListWorkflows(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to list workflows")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list workflows"))
	}

	return connect.NewResponse(&workflowv1.ListWorkflowsResponse{
		Workflows: workflows,
	}), nil
}

// CreateWorkflow persists a new workflow from a raw YAML document — see
// workflow_write.go's createWorkflow, shared with ui.go's New-workflow form
// handler.
func (h *handler) CreateWorkflow(ctx context.Context, req *connect.Request[workflowv1.CreateWorkflowRequest]) (*connect.Response[workflowv1.CreateWorkflowResponse], error) {
	w, err := createWorkflow(ctx, h.store, req.Msg.GetYaml())
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkflowAlreadyExists):
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		case errors.Is(err, ErrWorkflowNotFound):
			// Unreachable via createWorkflow's own logic (it only returns this
			// for a store error other than "already exists"), kept for switch
			// exhaustiveness against workflowWriteStore's documented errors.
			return nil, connect.NewError(connect.CodeInternal, err)
		default:
			// Any other error is ParseWorkflow's own validation failure (bad
			// YAML, an unrecognized uses: scheme, a cyclic depends_on, ...) —
			// the caller's input was wrong, not a server problem.
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return connect.NewResponse(&workflowv1.CreateWorkflowResponse{Workflow: w}), nil
}

// UpdateWorkflow replaces an existing workflow's definition — see
// workflow_write.go's updateWorkflow, shared with ui.go's Edit-workflow form
// handler.
func (h *handler) UpdateWorkflow(ctx context.Context, req *connect.Request[workflowv1.UpdateWorkflowRequest]) (*connect.Response[workflowv1.UpdateWorkflowResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	w, err := updateWorkflow(ctx, h.store, name, req.Msg.GetYaml())
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkflowNotFound):
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("workflow %q not found", name))
		case errors.Is(err, ErrWorkflowNameMismatch):
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	return connect.NewResponse(&workflowv1.UpdateWorkflowResponse{Workflow: w}), nil
}

// DeleteWorkflow removes a workflow's definition. Its run history is left
// alone — see store.go's DeleteWorkflow.
func (h *handler) DeleteWorkflow(ctx context.Context, req *connect.Request[workflowv1.DeleteWorkflowRequest]) (*connect.Response[workflowv1.DeleteWorkflowResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	if err := h.store.DeleteWorkflow(ctx, name); err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("workflow %q not found", name))
		}
		h.logger.WithError(err).WithField("workflow", name).Error("failed to delete workflow")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete workflow"))
	}
	return connect.NewResponse(&workflowv1.DeleteWorkflowResponse{}), nil
}
