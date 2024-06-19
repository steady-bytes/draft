// @generated by protoc-gen-es v1.6.0 with parameter "target=js"
// @generated from file core/registry/service_discovery/v1/models.proto (package core.registry.service_discovery.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { proto3, Timestamp } from "@bufbuild/protobuf";

/**
 * Server currently falls into a category that is consuming requests from the outside world
 * the `Job` is something that is private and not serving any external requests. I could however
 * be pulling messages from a message queue, and or doing some batch processing. I.e. some sort of 
 * training.
 *
 * @generated from enum core.registry.service_discovery.v1.ProcessKind
 */
export const ProcessKind = proto3.makeEnum(
  "core.registry.service_discovery.v1.ProcessKind",
  [
    {no: 0, name: "INVALID_PROCESS_KIND"},
    {no: 1, name: "SERVER_PROCESS"},
    {no: 2, name: "JOB_PROCESS"},
  ],
);

/**
 * @generated from enum core.registry.service_discovery.v1.ProcessRunningState
 */
export const ProcessRunningState = proto3.makeEnum(
  "core.registry.service_discovery.v1.ProcessRunningState",
  [
    {no: 0, name: "INVALID_PROCESS_RUNNING_STATE"},
    {no: 1, name: "PROCESS_STARTING"},
    {no: 2, name: "PROCESS_TESTING"},
    {no: 3, name: "PROCESS_RUNNING"},
    {no: 4, name: "PROCESS_DICONNECTED"},
  ],
);

/**
 * @generated from enum core.registry.service_discovery.v1.ProcessHealthState
 */
export const ProcessHealthState = proto3.makeEnum(
  "core.registry.service_discovery.v1.ProcessHealthState",
  [
    {no: 0, name: "INVALID_PROCESS_HEALTH_STATE"},
    {no: 1, name: "PROCESS_HEALTHY"},
    {no: 2, name: "PROCESS_UNHEALTHY"},
  ],
);

/**
 * Entities
 *
 * @generated from message core.registry.service_discovery.v1.Zone
 */
export const Zone = proto3.makeMessageType(
  "core.registry.service_discovery.v1.Zone",
  [],
);

/**
 * ProcessIdentity - 
 *
 * @generated from message core.registry.service_discovery.v1.ProcessIdentity
 */
export const ProcessIdentity = proto3.makeMessageType(
  "core.registry.service_discovery.v1.ProcessIdentity",
  () => [
    { no: 1, name: "pid", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "registry_address", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 3, name: "token", kind: "message", T: Token },
  ],
);

/**
 * configuration the registry is giving to the process to run
 *
 * @generated from message core.registry.service_discovery.v1.StartupConfiguration
 */
export const StartupConfiguration = proto3.makeMessageType(
  "core.registry.service_discovery.v1.StartupConfiguration",
  () => [
    { no: 1, name: "assigned_port", kind: "scalar", T: 13 /* ScalarType.UINT32 */ },
  ],
);

/**
 * A process is a running program on a computer.
 *
 * @generated from message core.registry.service_discovery.v1.Process
 */
export const Process = proto3.makeMessageType(
  "core.registry.service_discovery.v1.Process",
  () => [
    { no: 1, name: "pid", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "name", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 3, name: "group", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 4, name: "local", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 5, name: "ip_address", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 6, name: "process_kind", kind: "enum", T: proto3.getEnumType(ProcessKind) },
    { no: 7, name: "metadata", kind: "message", T: Metadata, repeated: true },
    { no: 8, name: "location", kind: "message", T: GeoPoint },
    { no: 9, name: "joined_time", kind: "message", T: Timestamp },
    { no: 10, name: "left_time", kind: "message", T: Timestamp },
    { no: 11, name: "last_status_time", kind: "message", T: Timestamp },
    { no: 12, name: "running_state", kind: "enum", T: proto3.getEnumType(ProcessRunningState) },
    { no: 13, name: "health_state", kind: "enum", T: proto3.getEnumType(ProcessHealthState) },
    { no: 14, name: "token", kind: "message", T: Token },
  ],
);

/**
 * Associated data that can be used to lookup the process
 *
 * @generated from message core.registry.service_discovery.v1.Metadata
 */
export const Metadata = proto3.makeMessageType(
  "core.registry.service_discovery.v1.Metadata",
  () => [
    { no: 1, name: "pid", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "key", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 3, name: "value", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ],
);

/**
 * GeoPoint - Is the location of something using standard lat/lng notion.
 *
 * @generated from message core.registry.service_discovery.v1.GeoPoint
 */
export const GeoPoint = proto3.makeMessageType(
  "core.registry.service_discovery.v1.GeoPoint",
  () => [
    { no: 1, name: "lat", kind: "scalar", T: 2 /* ScalarType.FLOAT */ },
    { no: 2, name: "lng", kind: "scalar", T: 2 /* ScalarType.FLOAT */ },
  ],
);

/**
 * Token that is generated when the `Init` function is called with the correct `nonce`
 *
 * @generated from message core.registry.service_discovery.v1.Token
 */
export const Token = proto3.makeMessageType(
  "core.registry.service_discovery.v1.Token",
  () => [
    { no: 1, name: "id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 3, name: "jwt", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ],
);

