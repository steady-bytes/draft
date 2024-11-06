// @generated by protoc-gen-es v1.6.0 with parameter "target=js"
// @generated from file core/message_broker/actors/v1/producer.proto (package core.message_broker.actors.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { proto3 } from "@bufbuild/protobuf";
import { CloudEvent } from "./models_pb.js";

/**
 * Send this `Message` to the other `Actors` in the system that are subscribed to this `Message`
 *
 * @generated from message core.message_broker.actors.v1.ProduceRequest
 */
export const ProduceRequest = proto3.makeMessageType(
  "core.message_broker.actors.v1.ProduceRequest",
  () => [
    { no: 1, name: "message", kind: "message", T: CloudEvent },
  ],
);

/**
 * @generated from message core.message_broker.actors.v1.ProduceResponse
 */
export const ProduceResponse = proto3.makeMessageType(
  "core.message_broker.actors.v1.ProduceResponse",
  () => [
    { no: 1, name: "id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ],
);

