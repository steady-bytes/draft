// @generated by protoc-gen-connect-es v1.3.0 with parameter "target=js"
// @generated from file core/control_plane/networking/v1/service.proto (package core.control_plane.networking.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { AddRouteRequest, AddRouteResponse, DeleteRouteRequest, DeleteRouteResponse, ListRoutesRequest, ListRoutesResponse } from "./service_pb.js";
import { MethodKind } from "@bufbuild/protobuf";

/**
 * An interface to modify the networking configuration of the system. The networking configuration is primarly used to
 * make endpoints available outside of the system (public, or private). To efficiently route traffic to the correct process, the 
 * networking configuration is used to configure the `Envoy` proxy. The `Envoy` proxy is used to route traffic to the correct process
 * based on the provided `Route` configuration when the process registers to the system in this case `fuse`.
 *
 * 1. The `NetworkingService` is a `core` service implemented in `fuse`
 * 2. Alot of the networking configuration is specific for `Envoy`. The `Route` proto is an example of this. It's basically a one to one mapping
 *    of all fields that can't already be inferred by the `draft` framework.
 *
 *
 * @generated from service core.control_plane.networking.v1.NetworkingService
 */
export const NetworkingService = {
  typeName: "core.control_plane.networking.v1.NetworkingService",
  methods: {
    /**
     * Add route exposing an endpoint on the gateway and routing traffic to the correct process
     *
     * @generated from rpc core.control_plane.networking.v1.NetworkingService.AddRoute
     */
    addRoute: {
      name: "AddRoute",
      I: AddRouteRequest,
      O: AddRouteResponse,
      kind: MethodKind.Unary,
    },
    /**
     * List all routes in the networking configuration
     *
     * @generated from rpc core.control_plane.networking.v1.NetworkingService.ListRoutes
     */
    listRoutes: {
      name: "ListRoutes",
      I: ListRoutesRequest,
      O: ListRoutesResponse,
      kind: MethodKind.Unary,
    },
    /**
     * Delete a route from the networking configuration, requires the name of the route
     * and returns a response code to indicate the success of the operation
     *
     * @generated from rpc core.control_plane.networking.v1.NetworkingService.DeleteRoute
     */
    deleteRoute: {
      name: "DeleteRoute",
      I: DeleteRouteRequest,
      O: DeleteRouteResponse,
      kind: MethodKind.Unary,
    },
  }
};

