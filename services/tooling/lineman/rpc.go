// rpc.go: the only file that speaks connect.Request/Response and telemetry
// spans -- implements v1connect.LinemanServiceHandler by delegating to
// *controller (objective.go/task.go/agent.go/scheduler.go/loop.go), and
// wraps every mutating call in a chassis.StartSpan/SetBusinessAttribute/End
// pair so it produces a WideEvent carrying business context. See
// docs/architecture/lineman-implementation-plan.md's Telemetry section for
// why this can't just be chassis.NewTraceInterceptor()'s automatic span.
package main

import (
	"context"

	linemanv1 "github.com/steady-bytes/draft/api/tooling/lineman/v1"
	linemanv1Connect "github.com/steady-bytes/draft/api/tooling/lineman/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

type Rpc interface {
	chassis.RPCRegistrar
	linemanv1Connect.LinemanServiceHandler
}

type handler struct {
	logger chassis.Logger
	ctrl   *controller
}

func NewHandler(logger chassis.Logger, ctrl *controller) Rpc {
	return &handler{logger: logger, ctrl: ctrl}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, hh := linemanv1Connect.NewLinemanServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, hh, true)
}

// ─── Objectives ────────────────────────────────────────────────────────────

func (h *handler) CreateObjective(ctx context.Context, req *connect.Request[linemanv1.CreateObjectiveRequest]) (*connect.Response[linemanv1.Objective], error) {
	spanCtx, span := chassis.StartSpan(ctx, "objective.create")
	obj, err := h.ctrl.createObjective(spanCtx, req.Msg.GetName(), req.Msg.GetDescription(), req.Msg.GetStates())
	if err == nil {
		span.SetBusinessAttribute("objective_id", obj.GetId())
	}
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(obj), nil
}

func (h *handler) GetObjective(ctx context.Context, req *connect.Request[linemanv1.GetObjectiveRequest]) (*connect.Response[linemanv1.Objective], error) {
	obj, err := h.ctrl.getObjective(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(obj), nil
}

func (h *handler) ListObjectives(ctx context.Context, req *connect.Request[linemanv1.ListObjectivesRequest]) (*connect.Response[linemanv1.ListObjectivesResponse], error) {
	objs, err := h.ctrl.listObjectives(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&linemanv1.ListObjectivesResponse{Objectives: objs}), nil
}

// ─── Tasks ─────────────────────────────────────────────────────────────────

func (h *handler) CreateTask(ctx context.Context, req *connect.Request[linemanv1.CreateTaskRequest]) (*connect.Response[linemanv1.Task], error) {
	spanCtx, span := chassis.StartSpan(ctx, "task.create")
	span.SetBusinessAttribute("objective_id", req.Msg.GetObjectiveId())
	task, err := h.ctrl.createTask(spanCtx, req.Msg.GetObjectiveId(), req.Msg.GetName(), req.Msg.GetDetails(), req.Msg.GetPriority(), req.Msg.GetState())
	if err == nil {
		span.SetBusinessAttribute("task_id", task.GetId())
		span.SetBusinessAttribute("to_state", task.GetState())
	}
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(task), nil
}

func (h *handler) GetTask(ctx context.Context, req *connect.Request[linemanv1.GetTaskRequest]) (*connect.Response[linemanv1.Task], error) {
	task, err := h.ctrl.getTask(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(task), nil
}

func (h *handler) ListTasks(ctx context.Context, req *connect.Request[linemanv1.ListTasksRequest]) (*connect.Response[linemanv1.ListTasksResponse], error) {
	tasks, err := h.ctrl.listTasks(ctx, req.Msg.GetObjectiveId(), req.Msg.GetState(), req.Msg.GetAgentId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&linemanv1.ListTasksResponse{Tasks: tasks}), nil
}

func (h *handler) UpdateTaskState(ctx context.Context, req *connect.Request[linemanv1.UpdateTaskStateRequest]) (*connect.Response[linemanv1.Task], error) {
	spanCtx, span := chassis.StartSpan(ctx, "task.update_state")
	span.SetBusinessAttribute("task_id", req.Msg.GetTaskId())
	span.SetBusinessAttribute("to_state", req.Msg.GetNewState())
	task, fromState, err := h.ctrl.updateTaskState(spanCtx, req.Msg.GetTaskId(), req.Msg.GetNewState())
	if err == nil {
		span.SetBusinessAttribute("objective_id", task.GetObjectiveId())
		span.SetBusinessAttribute("from_state", fromState)
		h.ctrl.events.TaskStateChanged(task, fromState)
	}
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(task), nil
}

func (h *handler) ReorderTask(ctx context.Context, req *connect.Request[linemanv1.ReorderTaskRequest]) (*connect.Response[linemanv1.Task], error) {
	spanCtx, span := chassis.StartSpan(ctx, "task.reorder")
	span.SetBusinessAttribute("task_id", req.Msg.GetTaskId())
	task, err := h.ctrl.reorderTask(spanCtx, req.Msg.GetTaskId(), req.Msg.GetBeforeTaskId(), req.Msg.GetAfterTaskId())
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(task), nil
}

func (h *handler) AssignAgent(ctx context.Context, req *connect.Request[linemanv1.AssignAgentRequest]) (*connect.Response[linemanv1.Task], error) {
	spanCtx, span := chassis.StartSpan(ctx, "task.assign_agent")
	span.SetBusinessAttribute("task_id", req.Msg.GetTaskId())
	span.SetBusinessAttribute("agent_id", req.Msg.GetAgentId())
	task, err := h.ctrl.assignAgent(spanCtx, req.Msg.GetTaskId(), req.Msg.GetAgentId())
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(task), nil
}

// ─── Agents ────────────────────────────────────────────────────────────────

func (h *handler) Heartbeat(ctx context.Context, req *connect.Request[linemanv1.HeartbeatRequest]) (*connect.Response[linemanv1.HeartbeatResponse], error) {
	err := h.ctrl.heartbeat(ctx, req.Msg.GetAgentId(), req.Msg.GetAgentDisplayName(), req.Msg.GetAgentKind(), req.Msg.GetTaskId(), req.Msg.GetCurrentAction())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&linemanv1.HeartbeatResponse{}), nil
}

func (h *handler) ListAgents(ctx context.Context, req *connect.Request[linemanv1.ListAgentsRequest]) (*connect.Response[linemanv1.ListAgentsResponse], error) {
	agents, err := h.ctrl.listAgents(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&linemanv1.ListAgentsResponse{Agents: agents}), nil
}

// ─── Needs Input ────────────────────────────────────────────────────────────

func (h *handler) RequestInput(ctx context.Context, req *connect.Request[linemanv1.RequestInputRequest]) (*connect.Response[linemanv1.Task], error) {
	spanCtx, span := chassis.StartSpan(ctx, "task.request_input")
	span.SetBusinessAttribute("task_id", req.Msg.GetTaskId())
	task, err := h.ctrl.requestInput(spanCtx, req.Msg.GetTaskId(), req.Msg.GetQuestion(), req.Msg.GetOptions())
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(task), nil
}

func (h *handler) ProvideGuidance(ctx context.Context, req *connect.Request[linemanv1.ProvideGuidanceRequest]) (*connect.Response[linemanv1.Task], error) {
	spanCtx, span := chassis.StartSpan(ctx, "task.provide_guidance")
	span.SetBusinessAttribute("task_id", req.Msg.GetTaskId())
	span.SetBusinessAttribute("to_state", req.Msg.GetNextState())
	task, err := h.ctrl.provideGuidance(spanCtx, req.Msg.GetTaskId(), req.Msg.GetResponse(), req.Msg.GetNextState())
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(task), nil
}

// ─── Scheduler ──────────────────────────────────────────────────────────────

func (h *handler) CreateScheduledTask(ctx context.Context, req *connect.Request[linemanv1.CreateScheduledTaskRequest]) (*connect.Response[linemanv1.ScheduledTask], error) {
	spanCtx, span := chassis.StartSpan(ctx, "scheduled_task.create")
	span.SetBusinessAttribute("objective_id", req.Msg.GetObjectiveId())
	s, err := h.ctrl.createScheduledTask(spanCtx, req.Msg.GetObjectiveId(), req.Msg.GetDescription(), req.Msg.GetPriority(), req.Msg.GetFireAt())
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(s), nil
}

func (h *handler) ListScheduledTasks(ctx context.Context, req *connect.Request[linemanv1.ListScheduledTasksRequest]) (*connect.Response[linemanv1.ListScheduledTasksResponse], error) {
	scheduled, err := h.ctrl.listScheduledTasks(ctx, req.Msg.GetObjectiveId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&linemanv1.ListScheduledTasksResponse{ScheduledTasks: scheduled}), nil
}

func (h *handler) CancelScheduledTask(ctx context.Context, req *connect.Request[linemanv1.CancelScheduledTaskRequest]) (*connect.Response[linemanv1.CancelScheduledTaskResponse], error) {
	spanCtx, span := chassis.StartSpan(ctx, "scheduled_task.cancel")
	span.SetBusinessAttribute("scheduled_task_id", req.Msg.GetId())
	err := h.ctrl.cancelScheduledTask(spanCtx, req.Msg.GetId())
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&linemanv1.CancelScheduledTaskResponse{}), nil
}

// ─── Loops ──────────────────────────────────────────────────────────────────

func (h *handler) CreateLoop(ctx context.Context, req *connect.Request[linemanv1.CreateLoopRequest]) (*connect.Response[linemanv1.Loop], error) {
	spanCtx, span := chassis.StartSpan(ctx, "loop.create")
	span.SetBusinessAttribute("objective_id", req.Msg.GetObjectiveId())
	l, err := h.ctrl.createLoop(spanCtx, req.Msg.GetObjectiveId(), req.Msg.GetDescription(), req.Msg.GetPriority(), req.Msg.GetRecurrence())
	span.End(err)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(l), nil
}

func (h *handler) ListLoops(ctx context.Context, req *connect.Request[linemanv1.ListLoopsRequest]) (*connect.Response[linemanv1.ListLoopsResponse], error) {
	loops, err := h.ctrl.listLoops(ctx, req.Msg.GetObjectiveId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&linemanv1.ListLoopsResponse{Loops: loops}), nil
}

func (h *handler) PauseLoop(ctx context.Context, req *connect.Request[linemanv1.PauseLoopRequest]) (*connect.Response[linemanv1.Loop], error) {
	l, err := h.ctrl.setLoopStatus(ctx, req.Msg.GetId(), linemanv1.LoopStatus_LOOP_STATUS_PAUSED)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(l), nil
}

func (h *handler) ResumeLoop(ctx context.Context, req *connect.Request[linemanv1.ResumeLoopRequest]) (*connect.Response[linemanv1.Loop], error) {
	l, err := h.ctrl.setLoopStatus(ctx, req.Msg.GetId(), linemanv1.LoopStatus_LOOP_STATUS_ACTIVE)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(l), nil
}

func (h *handler) DeleteLoop(ctx context.Context, req *connect.Request[linemanv1.DeleteLoopRequest]) (*connect.Response[linemanv1.DeleteLoopResponse], error) {
	if err := h.ctrl.deleteLoop(ctx, req.Msg.GetId()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&linemanv1.DeleteLoopResponse{}), nil
}

// ─── Watch ──────────────────────────────────────────────────────────────────

func (h *handler) Watch(ctx context.Context, req *connect.Request[linemanv1.WatchRequest], stream *connect.ServerStream[linemanv1.WatchResponse]) error {
	ch, unsubscribe := h.ctrl.watchers.subscribe()
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil
		case resp, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}
