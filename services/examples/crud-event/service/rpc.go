package service

import (
	"context"
	"strconv"
	"time"

	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	crudv1Connect "github.com/steady-bytes/draft/api/examples/crud/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
)

type (
	Handler interface {
		chassis.RPCRegistrar
		crudv1Connect.CrudServiceHandler
	}
	handler struct {
		logger chassis.Logger
		model  Model
		events EventPublisher
	}
)

func NewHandler(logger chassis.Logger, model Model, events EventPublisher) Handler {
	return &handler{
		model:  model,
		logger: logger,
		events: events,
	}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := crudv1Connect.NewCrudServiceHandler(h, connect.WithInterceptors(chassis.NewTraceInterceptor()))
	server.AddHandler(pattern, handler, true)
}

// Each RPC below starts its own child span (chassis.StartSpan), a WideEvent in its own right, child
// of the span NewTraceInterceptor already opened for the request — same pattern
// services/core/blueprint/key_value/controller.go and services/core/fuse/control_plane/native's
// backend.go use, and services/tooling/bench/scheduler.go's worked example in
// docs/website/content/docs/architecture/beacon-observability.md. Splitting it out (rather than
// setting attributes on the automatic request-level span, which NewTraceInterceptor's path has no
// hook for) is what makes the business/runtime attributes and the log line below actually reach the
// WideEvent: chassis.SetBusinessAttribute/SetRuntimeAttribute and logger.WithContext(ctx) only
// attach to whichever span is on ctx, and only a StartSpan caller can set them.
//
// business_attributes carries the domain data this request is about; runtime_attributes carries the
// database call's own latency, infra telemetry rather than something the caller asked for. Both flow
// through Catalyst to Beacon as one WideEvent per call, queryable there by service_name = "crud-event".
//
// Beyond that WideEvent (this service's own observability side-channel), every RPC below also
// publishes a domain-level ModelEvent through h.events once its repository call succeeds — see
// service/events.go. The two are independent and serve different audiences: the WideEvent is
// this request's own trace, queryable in Beacon; the ModelEvent is a fact for anyone else in the
// cluster subscribed to examples.crud.v1.ModelEvent via Catalyst.Consume, unrelated to whether
// anyone is looking at this request's trace at all.

func (h *handler) Create(ctx context.Context, req *connect.Request[crudv1.CreateRequest]) (*connect.Response[crudv1.CreateResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "crud-event.create")
	span.SetBusinessAttribute("first_name", req.Msg.GetName().GetFirstName())
	span.SetBusinessAttribute("last_name", req.Msg.GetName().GetLastName())

	dbStart := time.Now()
	name, err := h.model.Create(ctx, req.Msg.Name)
	span.SetRuntimeAttribute("db.duration_ms", strconv.FormatInt(time.Since(dbStart).Milliseconds(), 10))
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).Error("failed to create name")
		span.End(err)
		return nil, err
	}
	span.SetBusinessAttribute("name_id", name.GetId())
	h.logger.WithContext(ctx).WithField("name_id", name.GetId()).Info("created name")
	span.End(nil)

	h.events.Publish(ctx, crudv1.Operation_OPERATION_CREATE, name)

	return connect.NewResponse(&crudv1.CreateResponse{
		Id: name.GetId(),
	}), nil
}

func (h *handler) Read(ctx context.Context, req *connect.Request[crudv1.ReadRequest]) (*connect.Response[crudv1.ReadResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "crud-event.read")
	span.SetBusinessAttribute("name_id", req.Msg.GetId())

	dbStart := time.Now()
	name, err := h.model.Read(ctx, req.Msg.Id)
	span.SetRuntimeAttribute("db.duration_ms", strconv.FormatInt(time.Since(dbStart).Milliseconds(), 10))
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).WithField("name_id", req.Msg.GetId()).Error("failed to read name")
		span.End(err)
		return nil, err
	}
	h.logger.WithContext(ctx).WithField("name_id", req.Msg.GetId()).Info("read name")
	span.End(nil)

	h.events.Publish(ctx, crudv1.Operation_OPERATION_READ, name)

	return connect.NewResponse(&crudv1.ReadResponse{
		Name: name,
	}), nil
}

func (h *handler) Update(ctx context.Context, req *connect.Request[crudv1.UpdateRequest]) (*connect.Response[crudv1.UpdateResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "crud-event.update")
	span.SetBusinessAttribute("name_id", req.Msg.GetName().GetId())
	span.SetBusinessAttribute("first_name", req.Msg.GetName().GetFirstName())
	span.SetBusinessAttribute("last_name", req.Msg.GetName().GetLastName())

	dbStart := time.Now()
	name, err := h.model.Update(ctx, req.Msg.Name)
	span.SetRuntimeAttribute("db.duration_ms", strconv.FormatInt(time.Since(dbStart).Milliseconds(), 10))
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).WithField("name_id", req.Msg.GetName().GetId()).Error("failed to update name")
		span.End(err)
		return nil, err
	}
	h.logger.WithContext(ctx).WithField("name_id", name.GetId()).Info("updated name")
	span.End(nil)

	h.events.Publish(ctx, crudv1.Operation_OPERATION_UPDATE, name)

	return connect.NewResponse(&crudv1.UpdateResponse{
		Id: name.GetId(),
	}), nil
}

func (h *handler) Delete(ctx context.Context, req *connect.Request[crudv1.DeleteRequest]) (*connect.Response[crudv1.DeleteResponse], error) {
	ctx, span := chassis.StartSpan(ctx, "crud-event.delete")
	span.SetBusinessAttribute("name_id", req.Msg.GetId())

	dbStart := time.Now()
	name, err := h.model.Delete(ctx, req.Msg.Id)
	span.SetRuntimeAttribute("db.duration_ms", strconv.FormatInt(time.Since(dbStart).Milliseconds(), 10))
	if err != nil {
		h.logger.WithContext(ctx).WithError(err).WithField("name_id", req.Msg.GetId()).Error("failed to delete name")
		span.End(err)
		return nil, err
	}
	h.logger.WithContext(ctx).WithField("name_id", req.Msg.GetId()).Info("deleted name")
	span.End(nil)

	h.events.Publish(ctx, crudv1.Operation_OPERATION_DELETE, name)

	return connect.NewResponse(&crudv1.DeleteResponse{
		Id: req.Msg.Id,
	}), nil
}
