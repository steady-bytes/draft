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

export class JournalQueryRequest extends jspb.Message {
  getQuery(): Query | undefined;
  setQuery(value?: Query): JournalQueryRequest;
  hasQuery(): boolean;
  clearQuery(): JournalQueryRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): JournalQueryRequest.AsObject;
  static toObject(includeInstance: boolean, msg: JournalQueryRequest): JournalQueryRequest.AsObject;
  static serializeBinaryToWriter(message: JournalQueryRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): JournalQueryRequest;
  static deserializeBinaryFromReader(message: JournalQueryRequest, reader: jspb.BinaryReader): JournalQueryRequest;
}

export namespace JournalQueryRequest {
  export type AsObject = {
    query?: Query.AsObject,
  }
}

export class JournalQueryResponse extends jspb.Message {
  getResultMap(): jspb.Map<string, Process>;
  clearResultMap(): JournalQueryResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): JournalQueryResponse.AsObject;
  static toObject(includeInstance: boolean, msg: JournalQueryResponse): JournalQueryResponse.AsObject;
  static serializeBinaryToWriter(message: JournalQueryResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): JournalQueryResponse;
  static deserializeBinaryFromReader(message: JournalQueryResponse, reader: jspb.BinaryReader): JournalQueryResponse;
}

export namespace JournalQueryResponse {
  export type AsObject = {
    resultMap: Array<[string, Process.AsObject]>,
  }
}

export class MonitorRequest extends jspb.Message {
  getLookUp(): Query | undefined;
  setLookUp(value?: Query): MonitorRequest;
  hasLookUp(): boolean;
  clearLookUp(): MonitorRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): MonitorRequest.AsObject;
  static toObject(includeInstance: boolean, msg: MonitorRequest): MonitorRequest.AsObject;
  static serializeBinaryToWriter(message: MonitorRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): MonitorRequest;
  static deserializeBinaryFromReader(message: MonitorRequest, reader: jspb.BinaryReader): MonitorRequest;
}

export namespace MonitorRequest {
  export type AsObject = {
    lookUp?: Query.AsObject,
  }
}

export class Query extends jspb.Message {
  getId(): string;
  setId(value: string): Query;

  getGroup(): string;
  setGroup(value: string): Query;

  getAll(): string;
  setAll(value: string): Query;

  getOptionCase(): Query.OptionCase;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Query.AsObject;
  static toObject(includeInstance: boolean, msg: Query): Query.AsObject;
  static serializeBinaryToWriter(message: Query, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Query;
  static deserializeBinaryFromReader(message: Query, reader: jspb.BinaryReader): Query;
}

export namespace Query {
  export type AsObject = {
    id: string,
    group: string,
    all: string,
  }

  export enum OptionCase { 
    OPTION_NOT_SET = 0,
    ID = 1,
    GROUP = 2,
    ALL = 3,
  }
}

export class RequestHandshake extends jspb.Message {
  getPayload(): Process | undefined;
  setPayload(value?: Process): RequestHandshake;
  hasPayload(): boolean;
  clearPayload(): RequestHandshake;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): RequestHandshake.AsObject;
  static toObject(includeInstance: boolean, msg: RequestHandshake): RequestHandshake.AsObject;
  static serializeBinaryToWriter(message: RequestHandshake, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): RequestHandshake;
  static deserializeBinaryFromReader(message: RequestHandshake, reader: jspb.BinaryReader): RequestHandshake;
}

export namespace RequestHandshake {
  export type AsObject = {
    payload?: Process.AsObject,
  }
}

export class Handshake extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): Handshake;

  getLeaderAddress(): string;
  setLeaderAddress(value: string): Handshake;

  getToken(): Token | undefined;
  setToken(value?: Token): Handshake;
  hasToken(): boolean;
  clearToken(): Handshake;

  getTransactionId(): string;
  setTransactionId(value: string): Handshake;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Handshake.AsObject;
  static toObject(includeInstance: boolean, msg: Handshake): Handshake.AsObject;
  static serializeBinaryToWriter(message: Handshake, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Handshake;
  static deserializeBinaryFromReader(message: Handshake, reader: jspb.BinaryReader): Handshake;
}

export namespace Handshake {
  export type AsObject = {
    processId: string,
    leaderAddress: string,
    token?: Token.AsObject,
    transactionId: string,
  }
}

export class ProcessDetails extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): ProcessDetails;

  getRunningState(): ProcessRunningState;
  setRunningState(value: ProcessRunningState): ProcessDetails;

  getHealthState(): ProcessHealthState;
  setHealthState(value: ProcessHealthState): ProcessDetails;

  getProcessKind(): ProcessKind;
  setProcessKind(value: ProcessKind): ProcessDetails;

  getToken(): string;
  setToken(value: string): ProcessDetails;

  getNonce(): string;
  setNonce(value: string): ProcessDetails;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ProcessDetails.AsObject;
  static toObject(includeInstance: boolean, msg: ProcessDetails): ProcessDetails.AsObject;
  static serializeBinaryToWriter(message: ProcessDetails, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ProcessDetails;
  static deserializeBinaryFromReader(message: ProcessDetails, reader: jspb.BinaryReader): ProcessDetails;
}

export namespace ProcessDetails {
  export type AsObject = {
    processId: string,
    runningState: ProcessRunningState,
    healthState: ProcessHealthState,
    processKind: ProcessKind,
    token: string,
    nonce: string,
  }
}

export class Empty extends jspb.Message {
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Empty.AsObject;
  static toObject(includeInstance: boolean, msg: Empty): Empty.AsObject;
  static serializeBinaryToWriter(message: Empty, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Empty;
  static deserializeBinaryFromReader(message: Empty, reader: jspb.BinaryReader): Empty;
}

export namespace Empty {
  export type AsObject = {
  }
}

export class DisconnectRequest extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): DisconnectRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): DisconnectRequest.AsObject;
  static toObject(includeInstance: boolean, msg: DisconnectRequest): DisconnectRequest.AsObject;
  static serializeBinaryToWriter(message: DisconnectRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): DisconnectRequest;
  static deserializeBinaryFromReader(message: DisconnectRequest, reader: jspb.BinaryReader): DisconnectRequest;
}

export namespace DisconnectRequest {
  export type AsObject = {
    processId: string,
  }
}

export class Disconnected extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): Disconnected;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Disconnected.AsObject;
  static toObject(includeInstance: boolean, msg: Disconnected): Disconnected.AsObject;
  static serializeBinaryToWriter(message: Disconnected, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Disconnected;
  static deserializeBinaryFromReader(message: Disconnected, reader: jspb.BinaryReader): Disconnected;
}

export namespace Disconnected {
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

  getTagsList(): Array<Metadata>;
  setTagsList(value: Array<Metadata>): Process;
  clearTagsList(): Process;
  addTags(value?: Metadata, index?: number): Metadata;

  getJoinedTime(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setJoinedTime(value?: google_protobuf_timestamp_pb.Timestamp): Process;
  hasJoinedTime(): boolean;
  clearJoinedTime(): Process;

  getLeftTime(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setLeftTime(value?: google_protobuf_timestamp_pb.Timestamp): Process;
  hasLeftTime(): boolean;
  clearLeftTime(): Process;

  getLastStatusTime(): google_protobuf_timestamp_pb.Timestamp | undefined;
  setLastStatusTime(value?: google_protobuf_timestamp_pb.Timestamp): Process;
  hasLastStatusTime(): boolean;
  clearLastStatusTime(): Process;

  getVersion(): string;
  setVersion(value: string): Process;

  getRunningState(): ProcessRunningState;
  setRunningState(value: ProcessRunningState): Process;

  getHealthState(): ProcessHealthState;
  setHealthState(value: ProcessHealthState): Process;

  getToken(): Token | undefined;
  setToken(value?: Token): Process;
  hasToken(): boolean;
  clearToken(): Process;

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
    tagsList: Array<Metadata.AsObject>,
    joinedTime?: google_protobuf_timestamp_pb.Timestamp.AsObject,
    leftTime?: google_protobuf_timestamp_pb.Timestamp.AsObject,
    lastStatusTime?: google_protobuf_timestamp_pb.Timestamp.AsObject,
    version: string,
    runningState: ProcessRunningState,
    healthState: ProcessHealthState,
    token?: Token.AsObject,
  }
}

export class Token extends jspb.Message {
  getId(): string;
  setId(value: string): Token;

  getJwt(): string;
  setJwt(value: string): Token;

  getNonce(): string;
  setNonce(value: string): Token;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): Token.AsObject;
  static toObject(includeInstance: boolean, msg: Token): Token.AsObject;
  static serializeBinaryToWriter(message: Token, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): Token;
  static deserializeBinaryFromReader(message: Token, reader: jspb.BinaryReader): Token;
}

export namespace Token {
  export type AsObject = {
    id: string,
    jwt: string,
    nonce: string,
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
export enum ProcessRunningState { 
  INVALID_PROCESS_RUNNING_STATE = 0,
  PROCESS_STARTING = 1,
  PROCESS_TESTING = 2,
  PROCESS_RUNNING = 3,
  PROCESS_DICONNECTED = 4,
}
export enum ProcessHealthState { 
  INVALID_PROCESS_HEALTH_STATE = 0,
  PROCESS_HEALTHY = 1,
  PROCESS_UNHEALTHY = 2,
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
