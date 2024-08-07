// @generated by protoc-gen-es v1.6.0 with parameter "target=ts"
// @generated from file core/control_plane/networking/v1/service.proto (package core.control_plane.networking.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import type { BinaryReadOptions, FieldList, JsonReadOptions, JsonValue, PartialMessage, PlainMessage } from "@bufbuild/protobuf";
import { Message, proto3 } from "@bufbuild/protobuf";

/**
 * @generated from enum core.control_plane.networking.v1.AddRouteResponseCode
 */
export enum AddRouteResponseCode {
  /**
   * @generated from enum value: INVALID_ADD_ROUTE_RESPONSE_CODE = 0;
   */
  INVALID_ADD_ROUTE_RESPONSE_CODE = 0,

  /**
   * @generated from enum value: OK = 1;
   */
  OK = 1,

  /**
   * @generated from enum value: ERROR = 2;
   */
  ERROR = 2,

  /**
   * @generated from enum value: INVALID_REQUEST = 3;
   */
  INVALID_REQUEST = 3,
}
// Retrieve enum metadata with: proto3.getEnumType(AddRouteResponseCode)
proto3.util.setEnumType(AddRouteResponseCode, "core.control_plane.networking.v1.AddRouteResponseCode", [
  { no: 0, name: "INVALID_ADD_ROUTE_RESPONSE_CODE" },
  { no: 1, name: "OK" },
  { no: 2, name: "ERROR" },
  { no: 3, name: "INVALID_REQUEST" },
]);

/**
 * @generated from enum core.control_plane.networking.v1.DeleteRouteCode
 */
export enum DeleteRouteCode {
  /**
   * @generated from enum value: INVALID_DELETE_ROUTE_RESPONSE_CODE = 0;
   */
  INVALID_DELETE_ROUTE_RESPONSE_CODE = 0,

  /**
   * @generated from enum value: DELETE_ROUTE_OK = 1;
   */
  DELETE_ROUTE_OK = 1,

  /**
   * @generated from enum value: DELETE_ROUTE_ERROR = 2;
   */
  DELETE_ROUTE_ERROR = 2,
}
// Retrieve enum metadata with: proto3.getEnumType(DeleteRouteCode)
proto3.util.setEnumType(DeleteRouteCode, "core.control_plane.networking.v1.DeleteRouteCode", [
  { no: 0, name: "INVALID_DELETE_ROUTE_RESPONSE_CODE" },
  { no: 1, name: "DELETE_ROUTE_OK" },
  { no: 2, name: "DELETE_ROUTE_ERROR" },
]);

/**
 * AddRouteRequest - Add a route to the networking configuration
 *
 * @generated from message core.control_plane.networking.v1.AddRouteRequest
 */
export class AddRouteRequest extends Message<AddRouteRequest> {
  /**
   * should this be the route type from envoy config. So that I can use the same proto
   * or should I make an internal proto and convert them back and forth
   * An advantage of using the envoy `Route` proto is that it's already ubuqitous in the
   * wild. A disadvantage is that it's a bit more complex than what I need.
   *
   * @generated from field: core.control_plane.networking.v1.Route route = 1;
   */
  route?: Route;

  constructor(data?: PartialMessage<AddRouteRequest>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.AddRouteRequest";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "route", kind: "message", T: Route },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): AddRouteRequest {
    return new AddRouteRequest().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): AddRouteRequest {
    return new AddRouteRequest().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): AddRouteRequest {
    return new AddRouteRequest().fromJsonString(jsonString, options);
  }

  static equals(a: AddRouteRequest | PlainMessage<AddRouteRequest> | undefined, b: AddRouteRequest | PlainMessage<AddRouteRequest> | undefined): boolean {
    return proto3.util.equals(AddRouteRequest, a, b);
  }
}

/**
 * AddRouteResponse - Response to adding a route to the networking configuration. Just because a message
 * was received doesn't mean it was successful. The `code` field is used to determine the success of the
 * route entry.
 *
 * @generated from message core.control_plane.networking.v1.AddRouteResponse
 */
export class AddRouteResponse extends Message<AddRouteResponse> {
  /**
   * @generated from field: core.control_plane.networking.v1.AddRouteResponseCode code = 1;
   */
  code = AddRouteResponseCode.INVALID_ADD_ROUTE_RESPONSE_CODE;

  constructor(data?: PartialMessage<AddRouteResponse>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.AddRouteResponse";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "code", kind: "enum", T: proto3.getEnumType(AddRouteResponseCode) },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): AddRouteResponse {
    return new AddRouteResponse().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): AddRouteResponse {
    return new AddRouteResponse().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): AddRouteResponse {
    return new AddRouteResponse().fromJsonString(jsonString, options);
  }

  static equals(a: AddRouteResponse | PlainMessage<AddRouteResponse> | undefined, b: AddRouteResponse | PlainMessage<AddRouteResponse> | undefined): boolean {
    return proto3.util.equals(AddRouteResponse, a, b);
  }
}

/**
 * @generated from message core.control_plane.networking.v1.ListRoutesRequest
 */
export class ListRoutesRequest extends Message<ListRoutesRequest> {
  constructor(data?: PartialMessage<ListRoutesRequest>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.ListRoutesRequest";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): ListRoutesRequest {
    return new ListRoutesRequest().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): ListRoutesRequest {
    return new ListRoutesRequest().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): ListRoutesRequest {
    return new ListRoutesRequest().fromJsonString(jsonString, options);
  }

  static equals(a: ListRoutesRequest | PlainMessage<ListRoutesRequest> | undefined, b: ListRoutesRequest | PlainMessage<ListRoutesRequest> | undefined): boolean {
    return proto3.util.equals(ListRoutesRequest, a, b);
  }
}

/**
 * @generated from message core.control_plane.networking.v1.ListRoutesResponse
 */
export class ListRoutesResponse extends Message<ListRoutesResponse> {
  /**
   * @generated from field: repeated core.control_plane.networking.v1.Route routes = 1;
   */
  routes: Route[] = [];

  constructor(data?: PartialMessage<ListRoutesResponse>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.ListRoutesResponse";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "routes", kind: "message", T: Route, repeated: true },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): ListRoutesResponse {
    return new ListRoutesResponse().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): ListRoutesResponse {
    return new ListRoutesResponse().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): ListRoutesResponse {
    return new ListRoutesResponse().fromJsonString(jsonString, options);
  }

  static equals(a: ListRoutesResponse | PlainMessage<ListRoutesResponse> | undefined, b: ListRoutesResponse | PlainMessage<ListRoutesResponse> | undefined): boolean {
    return proto3.util.equals(ListRoutesResponse, a, b);
  }
}

/**
 * @generated from message core.control_plane.networking.v1.DeleteRouteRequest
 */
export class DeleteRouteRequest extends Message<DeleteRouteRequest> {
  /**
   * route names must be unique making name the primary identifier of a route
   *
   * @generated from field: string name = 1;
   */
  name = "";

  constructor(data?: PartialMessage<DeleteRouteRequest>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.DeleteRouteRequest";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "name", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): DeleteRouteRequest {
    return new DeleteRouteRequest().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): DeleteRouteRequest {
    return new DeleteRouteRequest().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): DeleteRouteRequest {
    return new DeleteRouteRequest().fromJsonString(jsonString, options);
  }

  static equals(a: DeleteRouteRequest | PlainMessage<DeleteRouteRequest> | undefined, b: DeleteRouteRequest | PlainMessage<DeleteRouteRequest> | undefined): boolean {
    return proto3.util.equals(DeleteRouteRequest, a, b);
  }
}

/**
 * @generated from message core.control_plane.networking.v1.DeleteRouteResponse
 */
export class DeleteRouteResponse extends Message<DeleteRouteResponse> {
  /**
   * @generated from field: core.control_plane.networking.v1.DeleteRouteCode code = 1;
   */
  code = DeleteRouteCode.INVALID_DELETE_ROUTE_RESPONSE_CODE;

  constructor(data?: PartialMessage<DeleteRouteResponse>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.DeleteRouteResponse";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "code", kind: "enum", T: proto3.getEnumType(DeleteRouteCode) },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): DeleteRouteResponse {
    return new DeleteRouteResponse().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): DeleteRouteResponse {
    return new DeleteRouteResponse().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): DeleteRouteResponse {
    return new DeleteRouteResponse().fromJsonString(jsonString, options);
  }

  static equals(a: DeleteRouteResponse | PlainMessage<DeleteRouteResponse> | undefined, b: DeleteRouteResponse | PlainMessage<DeleteRouteResponse> | undefined): boolean {
    return proto3.util.equals(DeleteRouteResponse, a, b);
  }
}

/**
 * Route - Close match to the `Route` proto in envoy. Anything that can't be inferred by the draft
 * framework needs to be added by the `process` adding the route configuration.
 *
 * The process will register individual routes, while cluster and virtual host configuration will be handled by the framework.
 * current integration is `process` -> `fuse` -> `envoy`
 *
 * @generated from message core.control_plane.networking.v1.Route
 */
export class Route extends Message<Route> {
  /**
   * Name for the route
   *
   * @generated from field: string name = 1;
   */
  name = "";

  /**
   * Route matching parameters
   *
   * @generated from field: core.control_plane.networking.v1.RouteMatch match = 2;
   */
  match?: RouteMatch;

  /**
   * Endpoint parameters
   *
   * @generated from field: core.control_plane.networking.v1.Endpoint endpoint = 3;
   */
  endpoint?: Endpoint;

  /**
   * EnableHTTP2 enables HTTP2 support
   *
   * @generated from field: bool enable_http2 = 4;
   */
  enableHttp2 = false;

  constructor(data?: PartialMessage<Route>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.Route";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "name", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "match", kind: "message", T: RouteMatch },
    { no: 3, name: "endpoint", kind: "message", T: Endpoint },
    { no: 4, name: "enable_http2", kind: "scalar", T: 8 /* ScalarType.BOOL */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): Route {
    return new Route().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): Route {
    return new Route().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): Route {
    return new Route().fromJsonString(jsonString, options);
  }

  static equals(a: Route | PlainMessage<Route> | undefined, b: Route | PlainMessage<Route> | undefined): boolean {
    return proto3.util.equals(Route, a, b);
  }
}

/**
 * parameters for the endpoint a route will map to
 *
 * @generated from message core.control_plane.networking.v1.Endpoint
 */
export class Endpoint extends Message<Endpoint> {
  /**
   * host represents the address of the endpoint (upstream). can be either a hostname or an ip address
   *
   * @generated from field: string host = 1;
   */
  host = "";

  /**
   * port represents the port on the host of the endpoint (upstream)
   *
   * @generated from field: uint32 port = 2;
   */
  port = 0;

  constructor(data?: PartialMessage<Endpoint>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.Endpoint";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "host", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "port", kind: "scalar", T: 13 /* ScalarType.UINT32 */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): Endpoint {
    return new Endpoint().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): Endpoint {
    return new Endpoint().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): Endpoint {
    return new Endpoint().fromJsonString(jsonString, options);
  }

  static equals(a: Endpoint | PlainMessage<Endpoint> | undefined, b: Endpoint | PlainMessage<Endpoint> | undefined): boolean {
    return proto3.util.equals(Endpoint, a, b);
  }
}

/**
 * parameters for matching a route
 *
 * @generated from message core.control_plane.networking.v1.RouteMatch
 */
export class RouteMatch extends Message<RouteMatch> {
  /**
   * domains for the url a configured in `fuse` but the path to be matched of a route is configured by the `process`
   * (ie. api.draft.com/health -> /health) 
   *
   * @generated from field: string prefix = 1;
   */
  prefix = "";

  /**
   * option to match headers of a request
   * TODO -> implement pre 1.0 relase of `fuse`
   *
   * @generated from field: optional core.control_plane.networking.v1.HeaderMatchOptions headers = 2;
   */
  headers?: HeaderMatchOptions;

  /**
   * options to simplify the matching of a route for grpc. Most request will be grpc and this configuration
   * makes that integration easier.
   * TODO -> implement pre 1.0 relase of `fuse`
   *
   * @generated from field: optional core.control_plane.networking.v1.GrpcMatchOptions grpc_match_options = 3;
   */
  grpcMatchOptions?: GrpcMatchOptions;

  /**
   * REF: Envoy
   * Specifies a set of dynamic metadata that a route must match.
   * The router will check the dynamic metadata against all the specified dynamic metadata matchers.
   * If the number of specified dynamic metadata matchers is nonzero, they all must match the
   * dynamic metadata for a match to occur.
   * TODO -> implement pre 2.0 relase of `fuse`
   *
   * @generated from field: optional core.control_plane.networking.v1.DynamicMetadata dynamic_metadata = 4;
   */
  dynamicMetadata?: DynamicMetadata;

  /**
   * Host address for the route
   *
   * @generated from field: string host = 5;
   */
  host = "";

  constructor(data?: PartialMessage<RouteMatch>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.RouteMatch";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "prefix", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "headers", kind: "message", T: HeaderMatchOptions, opt: true },
    { no: 3, name: "grpc_match_options", kind: "message", T: GrpcMatchOptions, opt: true },
    { no: 4, name: "dynamic_metadata", kind: "message", T: DynamicMetadata, opt: true },
    { no: 5, name: "host", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): RouteMatch {
    return new RouteMatch().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): RouteMatch {
    return new RouteMatch().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): RouteMatch {
    return new RouteMatch().fromJsonString(jsonString, options);
  }

  static equals(a: RouteMatch | PlainMessage<RouteMatch> | undefined, b: RouteMatch | PlainMessage<RouteMatch> | undefined): boolean {
    return proto3.util.equals(RouteMatch, a, b);
  }
}

/**
 * consider using the `key/value` from `blueprint` key/value store
 * TODO -> implement pre 1.0 relase of `fuse`
 *
 * @generated from message core.control_plane.networking.v1.HeaderMatchOptions
 */
export class HeaderMatchOptions extends Message<HeaderMatchOptions> {
  /**
   * @generated from field: string key = 1;
   */
  key = "";

  /**
   * @generated from field: string value = 2;
   */
  value = "";

  constructor(data?: PartialMessage<HeaderMatchOptions>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.HeaderMatchOptions";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "key", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "value", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): HeaderMatchOptions {
    return new HeaderMatchOptions().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): HeaderMatchOptions {
    return new HeaderMatchOptions().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): HeaderMatchOptions {
    return new HeaderMatchOptions().fromJsonString(jsonString, options);
  }

  static equals(a: HeaderMatchOptions | PlainMessage<HeaderMatchOptions> | undefined, b: HeaderMatchOptions | PlainMessage<HeaderMatchOptions> | undefined): boolean {
    return proto3.util.equals(HeaderMatchOptions, a, b);
  }
}

/**
 * GrpcMatchOptions - Options to simplify the matching of a route for grpc. Most request will be grpc and this configuration
 * should make the integration easier.
 * TODO -> implement pre 1.0 relase of `fuse`
 *
 * @generated from message core.control_plane.networking.v1.GrpcMatchOptions
 */
export class GrpcMatchOptions extends Message<GrpcMatchOptions> {
  constructor(data?: PartialMessage<GrpcMatchOptions>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.GrpcMatchOptions";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): GrpcMatchOptions {
    return new GrpcMatchOptions().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): GrpcMatchOptions {
    return new GrpcMatchOptions().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): GrpcMatchOptions {
    return new GrpcMatchOptions().fromJsonString(jsonString, options);
  }

  static equals(a: GrpcMatchOptions | PlainMessage<GrpcMatchOptions> | undefined, b: GrpcMatchOptions | PlainMessage<GrpcMatchOptions> | undefined): boolean {
    return proto3.util.equals(GrpcMatchOptions, a, b);
  }
}

/**
 * DynamicMetadata - Specifies a set of dynamic metadata that a route must match. Dynamic metadata can be used in a variety of ways
 * and is a powerful feature of envoy `fuse` will most likely use this feature to add additional information to the route.
 * TODO -> implement pre 2.0 relase of `fuse`
 *
 * @generated from message core.control_plane.networking.v1.DynamicMetadata
 */
export class DynamicMetadata extends Message<DynamicMetadata> {
  constructor(data?: PartialMessage<DynamicMetadata>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.control_plane.networking.v1.DynamicMetadata";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): DynamicMetadata {
    return new DynamicMetadata().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): DynamicMetadata {
    return new DynamicMetadata().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): DynamicMetadata {
    return new DynamicMetadata().fromJsonString(jsonString, options);
  }

  static equals(a: DynamicMetadata | PlainMessage<DynamicMetadata> | undefined, b: DynamicMetadata | PlainMessage<DynamicMetadata> | undefined): boolean {
    return proto3.util.equals(DynamicMetadata, a, b);
  }
}

