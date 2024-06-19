// @generated by protoc-gen-connect-query v1.1.3 with parameter "target=ts"
// @generated from file core/consensus/raft/v1/service.proto (package core.consensus.raft.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { MethodKind } from "@bufbuild/protobuf";
import { JoinRequest, JoinResponse, RemoveRequest, RemoveResponse, StatsRequest, StatsResponse } from "./service_pb.js";

/**
 * Join the raft cluster
 *
 * @generated from rpc core.consensus.raft.v1.RaftService.Join
 */
export const join = {
  localName: "join",
  name: "Join",
  kind: MethodKind.Unary,
  I: JoinRequest,
  O: JoinResponse,
  service: {
    typeName: "core.consensus.raft.v1.RaftService"
  }
} as const;

/**
 * Leave the raft cluster
 *
 * @generated from rpc core.consensus.raft.v1.RaftService.Remove
 */
export const remove = {
  localName: "remove",
  name: "Remove",
  kind: MethodKind.Unary,
  I: RemoveRequest,
  O: RemoveResponse,
  service: {
    typeName: "core.consensus.raft.v1.RaftService"
  }
} as const;

/**
 * Gather raft cluster stats
 *
 * @generated from rpc core.consensus.raft.v1.RaftService.Stats
 */
export const stats = {
  localName: "stats",
  name: "Stats",
  kind: MethodKind.Unary,
  I: StatsRequest,
  O: StatsResponse,
  service: {
    typeName: "core.consensus.raft.v1.RaftService"
  }
} as const;
