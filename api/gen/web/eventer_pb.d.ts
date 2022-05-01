import * as jspb from 'google-protobuf'

import * as google_protobuf_any_pb from 'google-protobuf/google/protobuf/any_pb';
import * as google_protobuf_timestamp_pb from 'google-protobuf/google/protobuf/timestamp_pb';
import * as validate_validate_pb from './validate/validate_pb';


export class Event extends jspb.Message {
  getId(): string;
  setId(value: string): Event;

  getAggregateId(): string;
  setAggregateId(value: string): Event;

  getTransactionId(): string;
  setTransactionId(value: string): Event;

  getData(): string;
  setData(value: string): Event;

  getCreatedAt(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setCreatedAt(value?: google_protobuf_timestamp_pb.Timestamp): Event;
  hasCreatedAt(): boolean;
  clearCreatedAt(): Event;

  getPublishedAt(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setPublishedAt(value?: google_protobuf_timestamp_pb.Timestamp): Event;
  hasPublishedAt(): boolean;
  clearPublishedAt(): Event;

  getAggregateKind(): AggregateKind;
  setAggregateKind(value: AggregateKind): Event;

  getEventCode(): EventCode;
  setEventCode(value: EventCode): Event;

  getSideAffect(): boolean;
  setSideAffect(value: boolean): Event;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Event.AsObject;
  static toObject(includeInstance: boolean, msg: Event): Event.AsObject;
  static serializeBinaryToWriter(message: Event, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Event;
  static deserializeBinaryFromReader(message: Event, reader: jspb.BinaryReader): Event;
}

export namespace Event {
  export type AsObject = {
    id: string,
    aggregateId: string,
    transactionId: string,
    data: string,
    createdAt?: google_protobuf_timestamp_pb.Timestamp.AsObject,
    publishedAt?: google_protobuf_timestamp_pb.Timestamp.AsObject,
    aggregateKind: AggregateKind,
    eventCode: EventCode,
    sideAffect: boolean,
  }
}

export class EmitEventRequest extends jspb.Message {
  getPayload(): Event | undefined;
  setPayload(value?: Event): EmitEventRequest;
  hasPayload(): boolean;
  clearPayload(): EmitEventRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): EmitEventRequest.AsObject;
  static toObject(includeInstance: boolean, msg: EmitEventRequest): EmitEventRequest.AsObject;
  static serializeBinaryToWriter(message: EmitEventRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): EmitEventRequest;
  static deserializeBinaryFromReader(message: EmitEventRequest, reader: jspb.BinaryReader): EmitEventRequest;
}

export namespace EmitEventRequest {
  export type AsObject = {
    payload?: Event.AsObject,
  }
}

export class EmitEventResponse extends jspb.Message {
  getResult(): Event | undefined;
  setResult(value?: Event): EmitEventResponse;
  hasResult(): boolean;
  clearResult(): EmitEventResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): EmitEventResponse.AsObject;
  static toObject(includeInstance: boolean, msg: EmitEventResponse): EmitEventResponse.AsObject;
  static serializeBinaryToWriter(message: EmitEventResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): EmitEventResponse;
  static deserializeBinaryFromReader(message: EmitEventResponse, reader: jspb.BinaryReader): EmitEventResponse;
}

export namespace EmitEventResponse {
  export type AsObject = {
    result?: Event.AsObject,
  }
}

export class HandshakeInitiated extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): HandshakeInitiated;

  getLeaderAddress(): string;
  setLeaderAddress(value: string): HandshakeInitiated;

  getInitiatedTime(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setInitiatedTime(value?: google_protobuf_timestamp_pb.Timestamp): HandshakeInitiated;
  hasInitiatedTime(): boolean;
  clearInitiatedTime(): HandshakeInitiated;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): HandshakeInitiated.AsObject;
  static toObject(includeInstance: boolean, msg: HandshakeInitiated): HandshakeInitiated.AsObject;
  static serializeBinaryToWriter(message: HandshakeInitiated, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): HandshakeInitiated;
  static deserializeBinaryFromReader(message: HandshakeInitiated, reader: jspb.BinaryReader): HandshakeInitiated;
}

export namespace HandshakeInitiated {
  export type AsObject = {
    processId: string,
    leaderAddress: string,
    initiatedTime?: google_protobuf_timestamp_pb.Timestamp.AsObject,
  }
}

export class ProcessConnected extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): ProcessConnected;

  getConnectedAt(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setConnectedAt(value?: google_protobuf_timestamp_pb.Timestamp): ProcessConnected;
  hasConnectedAt(): boolean;
  clearConnectedAt(): ProcessConnected;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ProcessConnected.AsObject;
  static toObject(includeInstance: boolean, msg: ProcessConnected): ProcessConnected.AsObject;
  static serializeBinaryToWriter(message: ProcessConnected, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ProcessConnected;
  static deserializeBinaryFromReader(message: ProcessConnected, reader: jspb.BinaryReader): ProcessConnected;
}

export namespace ProcessConnected {
  export type AsObject = {
    processId: string,
    connectedAt?: google_protobuf_timestamp_pb.Timestamp.AsObject,
  }
}

export class ProcessDisconnected extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): ProcessDisconnected;

  getDisconnectedAt(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setDisconnectedAt(value?: google_protobuf_timestamp_pb.Timestamp): ProcessDisconnected;
  hasDisconnectedAt(): boolean;
  clearDisconnectedAt(): ProcessDisconnected;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ProcessDisconnected.AsObject;
  static toObject(includeInstance: boolean, msg: ProcessDisconnected): ProcessDisconnected.AsObject;
  static serializeBinaryToWriter(message: ProcessDisconnected, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ProcessDisconnected;
  static deserializeBinaryFromReader(message: ProcessDisconnected, reader: jspb.BinaryReader): ProcessDisconnected;
}

export namespace ProcessDisconnected {
  export type AsObject = {
    processId: string,
    disconnectedAt?: google_protobuf_timestamp_pb.Timestamp.AsObject,
  }
}

export enum AggregateKind { 
  INVALID_AGGREGATE = 0,
  REGISTRY = 1,
}
export enum EventCode { 
  INVALID_EVENT_CODE = 0,
  HANDSHAKE_INITIATED = 1,
  PROCESS_CONNECTED = 2,
  PROCESS_DISCONNECTED = 3,
}
