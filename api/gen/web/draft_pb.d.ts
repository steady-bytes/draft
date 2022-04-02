import * as jspb from 'google-protobuf'

import * as google_protobuf_any_pb from 'google-protobuf/google/protobuf/any_pb';
import * as google_protobuf_timestamp_pb from 'google-protobuf/google/protobuf/timestamp_pb';
import * as validate_validate_pb from './validate/validate_pb';
import * as gorm_options_pb from './gorm/options_pb';


export class Command extends jspb.Message {
  getName(): string;
  setName(value: string): Command;

  getArguments(): google_protobuf_any_pb.Any | undefined;
  setArguments(value?: google_protobuf_any_pb.Any): Command;
  hasArguments(): boolean;
  clearArguments(): Command;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Command.AsObject;
  static toObject(includeInstance: boolean, msg: Command): Command.AsObject;
  static serializeBinaryToWriter(message: Command, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Command;
  static deserializeBinaryFromReader(message: Command, reader: jspb.BinaryReader): Command;
}

export namespace Command {
  export type AsObject = {
    name: string,
    arguments?: google_protobuf_any_pb.Any.AsObject,
  }
}

export class Output extends jspb.Message {
  getTransactionId(): string;
  setTransactionId(value: string): Output;

  getAggregateId(): string;
  setAggregateId(value: string): Output;

  getResult(): google_protobuf_any_pb.Any | undefined;
  setResult(value?: google_protobuf_any_pb.Any): Output;
  hasResult(): boolean;
  clearResult(): Output;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Output.AsObject;
  static toObject(includeInstance: boolean, msg: Output): Output.AsObject;
  static serializeBinaryToWriter(message: Output, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Output;
  static deserializeBinaryFromReader(message: Output, reader: jspb.BinaryReader): Output;
}

export namespace Output {
  export type AsObject = {
    transactionId: string,
    aggregateId: string,
    result?: google_protobuf_any_pb.Any.AsObject,
  }
}

export class Transaction extends jspb.Message {
  getTransactionId(): string;
  setTransactionId(value: string): Transaction;

  getAggregateId(): string;
  setAggregateId(value: string): Transaction;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Transaction.AsObject;
  static toObject(includeInstance: boolean, msg: Transaction): Transaction.AsObject;
  static serializeBinaryToWriter(message: Transaction, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Transaction;
  static deserializeBinaryFromReader(message: Transaction, reader: jspb.BinaryReader): Transaction;
}

export namespace Transaction {
  export type AsObject = {
    transactionId: string,
    aggregateId: string,
  }
}

export class ReadAggreageByIDRequest extends jspb.Message {
  getAggregate(): string;
  setAggregate(value: string): ReadAggreageByIDRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ReadAggreageByIDRequest.AsObject;
  static toObject(includeInstance: boolean, msg: ReadAggreageByIDRequest): ReadAggreageByIDRequest.AsObject;
  static serializeBinaryToWriter(message: ReadAggreageByIDRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ReadAggreageByIDRequest;
  static deserializeBinaryFromReader(message: ReadAggreageByIDRequest, reader: jspb.BinaryReader): ReadAggreageByIDRequest;
}

export namespace ReadAggreageByIDRequest {
  export type AsObject = {
    aggregate: string,
  }
}

export class ReadAggregateByIDRespose extends jspb.Message {
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ReadAggregateByIDRespose.AsObject;
  static toObject(includeInstance: boolean, msg: ReadAggregateByIDRespose): ReadAggregateByIDRespose.AsObject;
  static serializeBinaryToWriter(message: ReadAggregateByIDRespose, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ReadAggregateByIDRespose;
  static deserializeBinaryFromReader(message: ReadAggregateByIDRespose, reader: jspb.BinaryReader): ReadAggregateByIDRespose;
}

export namespace ReadAggregateByIDRespose {
  export type AsObject = {
  }
}

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

export class CreateEventRequest extends jspb.Message {
  getPayload(): Event | undefined;
  setPayload(value?: Event): CreateEventRequest;
  hasPayload(): boolean;
  clearPayload(): CreateEventRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): CreateEventRequest.AsObject;
  static toObject(includeInstance: boolean, msg: CreateEventRequest): CreateEventRequest.AsObject;
  static serializeBinaryToWriter(message: CreateEventRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): CreateEventRequest;
  static deserializeBinaryFromReader(message: CreateEventRequest, reader: jspb.BinaryReader): CreateEventRequest;
}

export namespace CreateEventRequest {
  export type AsObject = {
    payload?: Event.AsObject,
  }
}

export class CreateEventResponse extends jspb.Message {
  getResult(): Event | undefined;
  setResult(value?: Event): CreateEventResponse;
  hasResult(): boolean;
  clearResult(): CreateEventResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): CreateEventResponse.AsObject;
  static toObject(includeInstance: boolean, msg: CreateEventResponse): CreateEventResponse.AsObject;
  static serializeBinaryToWriter(message: CreateEventResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): CreateEventResponse;
  static deserializeBinaryFromReader(message: CreateEventResponse, reader: jspb.BinaryReader): CreateEventResponse;
}

export namespace CreateEventResponse {
  export type AsObject = {
    result?: Event.AsObject,
  }
}

export enum SystemAggregateKind { 
  INVALID_SYSTEM_AGGREGATE = 0,
}
export enum SystemEventCode { 
  INVALID_SYSTEM_EVENT_CODE = 0,
}
export enum AggregateKind { 
  INVALID_AGGREGATE = 0,
}
export enum EventCode { 
  INVALID_EVENT_CODE = 0,
}
