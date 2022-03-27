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

export class JoinRequest extends jspb.Message {
  getPayload(): Process | undefined;
  setPayload(value?: Process): JoinRequest;
  hasPayload(): boolean;
  clearPayload(): JoinRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): JoinRequest.AsObject;
  static toObject(includeInstance: boolean, msg: JoinRequest): JoinRequest.AsObject;
  static serializeBinaryToWriter(message: JoinRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): JoinRequest;
  static deserializeBinaryFromReader(message: JoinRequest, reader: jspb.BinaryReader): JoinRequest;
}

export namespace JoinRequest {
  export type AsObject = {
    payload?: Process.AsObject,
  }
}

export class JoinResponse extends jspb.Message {
  getResult(): Process | undefined;
  setResult(value?: Process): JoinResponse;
  hasResult(): boolean;
  clearResult(): JoinResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): JoinResponse.AsObject;
  static toObject(includeInstance: boolean, msg: JoinResponse): JoinResponse.AsObject;
  static serializeBinaryToWriter(message: JoinResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): JoinResponse;
  static deserializeBinaryFromReader(message: JoinResponse, reader: jspb.BinaryReader): JoinResponse;
}

export namespace JoinResponse {
  export type AsObject = {
    result?: Process.AsObject,
  }
}

export class LeaveRequest extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): LeaveRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): LeaveRequest.AsObject;
  static toObject(includeInstance: boolean, msg: LeaveRequest): LeaveRequest.AsObject;
  static serializeBinaryToWriter(message: LeaveRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): LeaveRequest;
  static deserializeBinaryFromReader(message: LeaveRequest, reader: jspb.BinaryReader): LeaveRequest;
}

export namespace LeaveRequest {
  export type AsObject = {
    processId: string,
  }
}

export class LeaveResponse extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): LeaveResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): LeaveResponse.AsObject;
  static toObject(includeInstance: boolean, msg: LeaveResponse): LeaveResponse.AsObject;
  static serializeBinaryToWriter(message: LeaveResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): LeaveResponse;
  static deserializeBinaryFromReader(message: LeaveResponse, reader: jspb.BinaryReader): LeaveResponse;
}

export namespace LeaveResponse {
  export type AsObject = {
    processId: string,
  }
}

export class Process extends jspb.Message {
  getId(): string;
  setId(value: string): Process;

  getName(): string;
  setName(value: string): Process;

  getGroup(): string;
  setGroup(value: string): Process;

  getLocal(): string;
  setLocal(value: string): Process;

  getIpAddress(): string;
  setIpAddress(value: string): Process;

  getProcessKind(): ProcessKind;
  setProcessKind(value: ProcessKind): Process;

  getMetadataList(): Array<Metadata>;
  setMetadataList(value: Array<Metadata>): Process;
  clearMetadataList(): Process;
  addMetadata(value?: Metadata, index?: number): Metadata;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Process.AsObject;
  static toObject(includeInstance: boolean, msg: Process): Process.AsObject;
  static serializeBinaryToWriter(message: Process, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Process;
  static deserializeBinaryFromReader(message: Process, reader: jspb.BinaryReader): Process;
}

export namespace Process {
  export type AsObject = {
    id: string,
    name: string,
    group: string,
    local: string,
    ipAddress: string,
    processKind: ProcessKind,
    metadataList: Array<Metadata.AsObject>,
  }
}

export class Metadata extends jspb.Message {
  getId(): string;
  setId(value: string): Metadata;

  getKey(): string;
  setKey(value: string): Metadata;

  getValue(): string;
  setValue(value: string): Metadata;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Metadata.AsObject;
  static toObject(includeInstance: boolean, msg: Metadata): Metadata.AsObject;
  static serializeBinaryToWriter(message: Metadata, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Metadata;
  static deserializeBinaryFromReader(message: Metadata, reader: jspb.BinaryReader): Metadata;
}

export namespace Metadata {
  export type AsObject = {
    id: string,
    key: string,
    value: string,
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

export enum ProcessKind { 
  INVALID_PROCESS_KIND = 0,
  AGGREGATE_PROCESS = 1,
  CONSUMER_PROCESS = 2,
  PROJECTION_PROCESS = 3,
  RPC_PROCESS = 4,
  HTTP_PROCESS = 5,
  DEFAULT_PROCESS = 6,
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
