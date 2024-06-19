// @generated by protoc-gen-es v1.6.0 with parameter "target=ts"
// @generated from file core/consensus/raft/v1/service.proto (package core.consensus.raft.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import type { BinaryReadOptions, FieldList, JsonReadOptions, JsonValue, PartialMessage, PlainMessage } from "@bufbuild/protobuf";
import { Message, proto3 } from "@bufbuild/protobuf";

/**
 * @generated from message core.consensus.raft.v1.JoinRequest
 */
export class JoinRequest extends Message<JoinRequest> {
  /**
   * @generated from field: string node_id = 1;
   */
  nodeId = "";

  /**
   * @generated from field: string raft_address = 2;
   */
  raftAddress = "";

  constructor(data?: PartialMessage<JoinRequest>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.consensus.raft.v1.JoinRequest";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "node_id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "raft_address", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): JoinRequest {
    return new JoinRequest().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): JoinRequest {
    return new JoinRequest().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): JoinRequest {
    return new JoinRequest().fromJsonString(jsonString, options);
  }

  static equals(a: JoinRequest | PlainMessage<JoinRequest> | undefined, b: JoinRequest | PlainMessage<JoinRequest> | undefined): boolean {
    return proto3.util.equals(JoinRequest, a, b);
  }
}

/**
 * @generated from message core.consensus.raft.v1.JoinResponse
 */
export class JoinResponse extends Message<JoinResponse> {
  /**
   * @generated from field: string node_id = 1;
   */
  nodeId = "";

  /**
   * @generated from field: string raft_address = 2;
   */
  raftAddress = "";

  constructor(data?: PartialMessage<JoinResponse>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.consensus.raft.v1.JoinResponse";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "node_id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "raft_address", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): JoinResponse {
    return new JoinResponse().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): JoinResponse {
    return new JoinResponse().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): JoinResponse {
    return new JoinResponse().fromJsonString(jsonString, options);
  }

  static equals(a: JoinResponse | PlainMessage<JoinResponse> | undefined, b: JoinResponse | PlainMessage<JoinResponse> | undefined): boolean {
    return proto3.util.equals(JoinResponse, a, b);
  }
}

/**
 * @generated from message core.consensus.raft.v1.RemoveRequest
 */
export class RemoveRequest extends Message<RemoveRequest> {
  /**
   * @generated from field: string node_id = 1;
   */
  nodeId = "";

  constructor(data?: PartialMessage<RemoveRequest>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.consensus.raft.v1.RemoveRequest";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "node_id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): RemoveRequest {
    return new RemoveRequest().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): RemoveRequest {
    return new RemoveRequest().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): RemoveRequest {
    return new RemoveRequest().fromJsonString(jsonString, options);
  }

  static equals(a: RemoveRequest | PlainMessage<RemoveRequest> | undefined, b: RemoveRequest | PlainMessage<RemoveRequest> | undefined): boolean {
    return proto3.util.equals(RemoveRequest, a, b);
  }
}

/**
 * @generated from message core.consensus.raft.v1.RemoveResponse
 */
export class RemoveResponse extends Message<RemoveResponse> {
  /**
   * @generated from field: string node_id = 1;
   */
  nodeId = "";

  constructor(data?: PartialMessage<RemoveResponse>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.consensus.raft.v1.RemoveResponse";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "node_id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): RemoveResponse {
    return new RemoveResponse().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): RemoveResponse {
    return new RemoveResponse().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): RemoveResponse {
    return new RemoveResponse().fromJsonString(jsonString, options);
  }

  static equals(a: RemoveResponse | PlainMessage<RemoveResponse> | undefined, b: RemoveResponse | PlainMessage<RemoveResponse> | undefined): boolean {
    return proto3.util.equals(RemoveResponse, a, b);
  }
}

/**
 * @generated from message core.consensus.raft.v1.StatsRequest
 */
export class StatsRequest extends Message<StatsRequest> {
  /**
   * @generated from field: string node_id = 1;
   */
  nodeId = "";

  constructor(data?: PartialMessage<StatsRequest>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.consensus.raft.v1.StatsRequest";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "node_id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): StatsRequest {
    return new StatsRequest().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): StatsRequest {
    return new StatsRequest().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): StatsRequest {
    return new StatsRequest().fromJsonString(jsonString, options);
  }

  static equals(a: StatsRequest | PlainMessage<StatsRequest> | undefined, b: StatsRequest | PlainMessage<StatsRequest> | undefined): boolean {
    return proto3.util.equals(StatsRequest, a, b);
  }
}

/**
 * @generated from message core.consensus.raft.v1.StatsResponse
 */
export class StatsResponse extends Message<StatsResponse> {
  /**
   * @generated from field: string node_id = 1;
   */
  nodeId = "";

  /**
   * @generated from field: core.consensus.raft.v1.Stats stats = 2;
   */
  stats?: Stats;

  constructor(data?: PartialMessage<StatsResponse>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.consensus.raft.v1.StatsResponse";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "node_id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "stats", kind: "message", T: Stats },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): StatsResponse {
    return new StatsResponse().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): StatsResponse {
    return new StatsResponse().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): StatsResponse {
    return new StatsResponse().fromJsonString(jsonString, options);
  }

  static equals(a: StatsResponse | PlainMessage<StatsResponse> | undefined, b: StatsResponse | PlainMessage<StatsResponse> | undefined): boolean {
    return proto3.util.equals(StatsResponse, a, b);
  }
}

/**
 * @generated from message core.consensus.raft.v1.Stats
 */
export class Stats extends Message<Stats> {
  /**
   * @generated from field: map<string, string> stats = 1;
   */
  stats: { [key: string]: string } = {};

  constructor(data?: PartialMessage<Stats>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.consensus.raft.v1.Stats";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "stats", kind: "map", K: 9 /* ScalarType.STRING */, V: {kind: "scalar", T: 9 /* ScalarType.STRING */} },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): Stats {
    return new Stats().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): Stats {
    return new Stats().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): Stats {
    return new Stats().fromJsonString(jsonString, options);
  }

  static equals(a: Stats | PlainMessage<Stats> | undefined, b: Stats | PlainMessage<Stats> | undefined): boolean {
    return proto3.util.equals(Stats, a, b);
  }
}

