package service

import (
	"context"

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
	}
)

func NewHandler(logger chassis.Logger, model Model) Handler {
	return &handler{
		model:  model,
		logger: logger,
	}
}

func (h *handler) RegisterRPC(server chassis.Rpcer) {
	pattern, handler := crudv1Connect.NewCrudServiceHandler(h)
	server.AddHandler(pattern, handler, true)
}

func (h *handler) Create(ctx context.Context, req *connect.Request[crudv1.CreateRequest]) (*connect.Response[crudv1.CreateResponse], error) {
	id, err := h.model.Create(ctx, req.Msg.Name)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&crudv1.CreateResponse{
		Id: id,
	}), nil
}

func (h *handler) Read(ctx context.Context, req *connect.Request[crudv1.ReadRequest]) (*connect.Response[crudv1.ReadResponse], error) {
	name, err := h.model.Read(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&crudv1.ReadResponse{
		Name: name,
	}), nil
}

func (h *handler) Update(ctx context.Context, req *connect.Request[crudv1.UpdateRequest]) (*connect.Response[crudv1.UpdateResponse], error) {
	id, err := h.model.Update(ctx, req.Msg.Name)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&crudv1.UpdateResponse{
		Id: id,
	}), nil
}

func (h *handler) Delete(ctx context.Context, req *connect.Request[crudv1.DeleteRequest]) (*connect.Response[crudv1.DeleteResponse], error) {
	err := h.model.Delete(ctx, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&crudv1.DeleteResponse{
		Id: req.Msg.Id,
	}), nil
}
