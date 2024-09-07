// @generated by protoc-gen-es v1.6.0 with parameter "target=js"
// @generated from file core/message_broker/actors/v1/consumer.proto (package core.message_broker.actors.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { proto3 } from "@bufbuild/protobuf";
import { Count, Message } from "./models_pb.js";

/**
 * @generated from message core.message_broker.actors.v1.ConsumeRequest
 */
export const ConsumeRequest = proto3.makeMessageType(
  "core.message_broker.actors.v1.ConsumeRequest",
  () => [
    { no: 1, name: "message", kind: "message", T: Message },
    { no: 2, name: "count", kind: "message", T: Count, opt: true },
  ],
);

/**
 * @generated from message core.message_broker.actors.v1.ConsumeResponse
 */
export const ConsumeResponse = proto3.makeMessageType(
  "core.message_broker.actors.v1.ConsumeResponse",
  () => [
    { no: 1, name: "message", kind: "message", T: Message },
  ],
);

