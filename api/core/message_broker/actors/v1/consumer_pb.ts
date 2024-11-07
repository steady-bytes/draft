// @generated by protoc-gen-es v1.6.0 with parameter "target=ts"
// @generated from file core/message_broker/actors/v1/consumer.proto (package core.message_broker.actors.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import type { BinaryReadOptions, FieldList, JsonReadOptions, JsonValue, PartialMessage, PlainMessage } from "@bufbuild/protobuf";
import { Message, proto3 } from "@bufbuild/protobuf";
import { CloudEvent } from "./models_pb.js";

/**
 * @generated from message core.message_broker.actors.v1.ConsumeRequest
 */
export class ConsumeRequest extends Message<ConsumeRequest> {
  /**
   * @generated from field: core.message_broker.actors.v1.CloudEvent message = 1;
   */
  message?: CloudEvent;

  constructor(data?: PartialMessage<ConsumeRequest>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.message_broker.actors.v1.ConsumeRequest";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "message", kind: "message", T: CloudEvent },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): ConsumeRequest {
    return new ConsumeRequest().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): ConsumeRequest {
    return new ConsumeRequest().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): ConsumeRequest {
    return new ConsumeRequest().fromJsonString(jsonString, options);
  }

  static equals(a: ConsumeRequest | PlainMessage<ConsumeRequest> | undefined, b: ConsumeRequest | PlainMessage<ConsumeRequest> | undefined): boolean {
    return proto3.util.equals(ConsumeRequest, a, b);
  }
}

/**
 * @generated from message core.message_broker.actors.v1.ConsumeResponse
 */
export class ConsumeResponse extends Message<ConsumeResponse> {
  /**
   * @generated from field: core.message_broker.actors.v1.CloudEvent message = 1;
   */
  message?: CloudEvent;

  constructor(data?: PartialMessage<ConsumeResponse>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.message_broker.actors.v1.ConsumeResponse";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "message", kind: "message", T: CloudEvent },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): ConsumeResponse {
    return new ConsumeResponse().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): ConsumeResponse {
    return new ConsumeResponse().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): ConsumeResponse {
    return new ConsumeResponse().fromJsonString(jsonString, options);
  }

  static equals(a: ConsumeResponse | PlainMessage<ConsumeResponse> | undefined, b: ConsumeResponse | PlainMessage<ConsumeResponse> | undefined): boolean {
    return proto3.util.equals(ConsumeResponse, a, b);
  }
}

