// @generated by protoc-gen-es v1.6.0 with parameter "target=ts"
// @generated from file core/message_broker/actors/v1/models.proto (package core.message_broker.actors.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import type { BinaryReadOptions, FieldList, JsonReadOptions, JsonValue, PartialMessage, PlainMessage } from "@bufbuild/protobuf";
import { Any, Message as Message$1, proto3, protoInt64 } from "@bufbuild/protobuf";

/**
 * Count is what determines the total order of the `Messages` in the system
 *
 * @generated from message core.message_broker.actors.v1.Count
 */
export class Count extends Message$1<Count> {
  /**
   * The page a message can be found on
   *
   * @generated from field: uint64 page = 1;
   */
  page = protoInt64.zero;

  /**
   * The number for that page
   *
   * @generated from field: uint64 number = 2;
   */
  number = protoInt64.zero;

  constructor(data?: PartialMessage<Count>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.message_broker.actors.v1.Count";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "page", kind: "scalar", T: 4 /* ScalarType.UINT64 */ },
    { no: 2, name: "number", kind: "scalar", T: 4 /* ScalarType.UINT64 */ },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): Count {
    return new Count().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): Count {
    return new Count().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): Count {
    return new Count().fromJsonString(jsonString, options);
  }

  static equals(a: Count | PlainMessage<Count> | undefined, b: Count | PlainMessage<Count> | undefined): boolean {
    return proto3.util.equals(Count, a, b);
  }
}

/**
 * Message is the main data structure that is sent between the `Producer` and the `Consumer`
 *
 * @generated from message core.message_broker.actors.v1.Message
 */
export class Message extends Message$1<Message> {
  /**
   * @generated from field: string id = 1;
   */
  id = "";

  /**
   * use this domain key, and send me all the messages 
   *
   * @generated from field: string domain = 2;
   */
  domain = "";

  /**
   * of this kind
   *
   * @generated from field: google.protobuf.Any kind = 3;
   */
  kind?: Any;

  /**
   * @generated from field: core.message_broker.actors.v1.Count count = 4;
   */
  count?: Count;

  constructor(data?: PartialMessage<Message>) {
    super();
    proto3.util.initPartial(data, this);
  }

  static readonly runtime: typeof proto3 = proto3;
  static readonly typeName = "core.message_broker.actors.v1.Message";
  static readonly fields: FieldList = proto3.util.newFieldList(() => [
    { no: 1, name: "id", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 2, name: "domain", kind: "scalar", T: 9 /* ScalarType.STRING */ },
    { no: 3, name: "kind", kind: "message", T: Any },
    { no: 4, name: "count", kind: "message", T: Count },
  ]);

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): Message {
    return new Message().fromBinary(bytes, options);
  }

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): Message {
    return new Message().fromJson(jsonValue, options);
  }

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): Message {
    return new Message().fromJsonString(jsonString, options);
  }

  static equals(a: Message | PlainMessage<Message> | undefined, b: Message | PlainMessage<Message> | undefined): boolean {
    return proto3.util.equals(Message, a, b);
  }
}

