// Code generated by protoc-gen-connect-go. DO NOT EDIT.
//
// Source: core/consensus/raft/v1/service.proto

package v1connect

import (
	connect "connectrpc.com/connect"
	context "context"
	errors "errors"
	v1 "github.com/steady-bytes/draft/api/core/consensus/raft/v1"
	http "net/http"
	strings "strings"
)

// This is a compile-time assertion to ensure that this generated file and the connect package are
// compatible. If you get a compiler error that this constant is not defined, this code was
// generated with a version of connect newer than the one compiled into your binary. You can fix the
// problem by either regenerating this code with an older version of connect or updating the connect
// version compiled into your binary.
const _ = connect.IsAtLeastVersion1_13_0

const (
	// RaftServiceName is the fully-qualified name of the RaftService service.
	RaftServiceName = "core.consensus.raft.v1.RaftService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// RaftServiceJoinProcedure is the fully-qualified name of the RaftService's Join RPC.
	RaftServiceJoinProcedure = "/core.consensus.raft.v1.RaftService/Join"
	// RaftServiceRemoveProcedure is the fully-qualified name of the RaftService's Remove RPC.
	RaftServiceRemoveProcedure = "/core.consensus.raft.v1.RaftService/Remove"
	// RaftServiceStatsProcedure is the fully-qualified name of the RaftService's Stats RPC.
	RaftServiceStatsProcedure = "/core.consensus.raft.v1.RaftService/Stats"
)

// These variables are the protoreflect.Descriptor objects for the RPCs defined in this package.
var (
	raftServiceServiceDescriptor      = v1.File_core_consensus_raft_v1_service_proto.Services().ByName("RaftService")
	raftServiceJoinMethodDescriptor   = raftServiceServiceDescriptor.Methods().ByName("Join")
	raftServiceRemoveMethodDescriptor = raftServiceServiceDescriptor.Methods().ByName("Remove")
	raftServiceStatsMethodDescriptor  = raftServiceServiceDescriptor.Methods().ByName("Stats")
)

// RaftServiceClient is a client for the core.consensus.raft.v1.RaftService service.
type RaftServiceClient interface {
	// Join the raft cluster
	Join(context.Context, *connect.Request[v1.JoinRequest]) (*connect.Response[v1.JoinResponse], error)
	// Leave the raft cluster
	Remove(context.Context, *connect.Request[v1.RemoveRequest]) (*connect.Response[v1.RemoveResponse], error)
	// Gather raft cluster stats
	Stats(context.Context, *connect.Request[v1.StatsRequest]) (*connect.Response[v1.StatsResponse], error)
}

// NewRaftServiceClient constructs a client for the core.consensus.raft.v1.RaftService service. By
// default, it uses the Connect protocol with the binary Protobuf Codec, asks for gzipped responses,
// and sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply the
// connect.WithGRPC() or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewRaftServiceClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) RaftServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &raftServiceClient{
		join: connect.NewClient[v1.JoinRequest, v1.JoinResponse](
			httpClient,
			baseURL+RaftServiceJoinProcedure,
			connect.WithSchema(raftServiceJoinMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		remove: connect.NewClient[v1.RemoveRequest, v1.RemoveResponse](
			httpClient,
			baseURL+RaftServiceRemoveProcedure,
			connect.WithSchema(raftServiceRemoveMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		stats: connect.NewClient[v1.StatsRequest, v1.StatsResponse](
			httpClient,
			baseURL+RaftServiceStatsProcedure,
			connect.WithSchema(raftServiceStatsMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
	}
}

// raftServiceClient implements RaftServiceClient.
type raftServiceClient struct {
	join   *connect.Client[v1.JoinRequest, v1.JoinResponse]
	remove *connect.Client[v1.RemoveRequest, v1.RemoveResponse]
	stats  *connect.Client[v1.StatsRequest, v1.StatsResponse]
}

// Join calls core.consensus.raft.v1.RaftService.Join.
func (c *raftServiceClient) Join(ctx context.Context, req *connect.Request[v1.JoinRequest]) (*connect.Response[v1.JoinResponse], error) {
	return c.join.CallUnary(ctx, req)
}

// Remove calls core.consensus.raft.v1.RaftService.Remove.
func (c *raftServiceClient) Remove(ctx context.Context, req *connect.Request[v1.RemoveRequest]) (*connect.Response[v1.RemoveResponse], error) {
	return c.remove.CallUnary(ctx, req)
}

// Stats calls core.consensus.raft.v1.RaftService.Stats.
func (c *raftServiceClient) Stats(ctx context.Context, req *connect.Request[v1.StatsRequest]) (*connect.Response[v1.StatsResponse], error) {
	return c.stats.CallUnary(ctx, req)
}

// RaftServiceHandler is an implementation of the core.consensus.raft.v1.RaftService service.
type RaftServiceHandler interface {
	// Join the raft cluster
	Join(context.Context, *connect.Request[v1.JoinRequest]) (*connect.Response[v1.JoinResponse], error)
	// Leave the raft cluster
	Remove(context.Context, *connect.Request[v1.RemoveRequest]) (*connect.Response[v1.RemoveResponse], error)
	// Gather raft cluster stats
	Stats(context.Context, *connect.Request[v1.StatsRequest]) (*connect.Response[v1.StatsResponse], error)
}

// NewRaftServiceHandler builds an HTTP handler from the service implementation. It returns the path
// on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewRaftServiceHandler(svc RaftServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	raftServiceJoinHandler := connect.NewUnaryHandler(
		RaftServiceJoinProcedure,
		svc.Join,
		connect.WithSchema(raftServiceJoinMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	raftServiceRemoveHandler := connect.NewUnaryHandler(
		RaftServiceRemoveProcedure,
		svc.Remove,
		connect.WithSchema(raftServiceRemoveMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	raftServiceStatsHandler := connect.NewUnaryHandler(
		RaftServiceStatsProcedure,
		svc.Stats,
		connect.WithSchema(raftServiceStatsMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	return "/core.consensus.raft.v1.RaftService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case RaftServiceJoinProcedure:
			raftServiceJoinHandler.ServeHTTP(w, r)
		case RaftServiceRemoveProcedure:
			raftServiceRemoveHandler.ServeHTTP(w, r)
		case RaftServiceStatsProcedure:
			raftServiceStatsHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedRaftServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedRaftServiceHandler struct{}

func (UnimplementedRaftServiceHandler) Join(context.Context, *connect.Request[v1.JoinRequest]) (*connect.Response[v1.JoinResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("core.consensus.raft.v1.RaftService.Join is not implemented"))
}

func (UnimplementedRaftServiceHandler) Remove(context.Context, *connect.Request[v1.RemoveRequest]) (*connect.Response[v1.RemoveResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("core.consensus.raft.v1.RaftService.Remove is not implemented"))
}

func (UnimplementedRaftServiceHandler) Stats(context.Context, *connect.Request[v1.StatsRequest]) (*connect.Response[v1.StatsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("core.consensus.raft.v1.RaftService.Stats is not implemented"))
}
