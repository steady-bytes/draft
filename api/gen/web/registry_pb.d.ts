import * as jspb from 'google-protobuf'

import * as google_protobuf_any_pb from 'google-protobuf/google/protobuf/any_pb';
import * as google_protobuf_timestamp_pb from 'google-protobuf/google/protobuf/timestamp_pb';
import * as validate_validate_pb from './validate/validate_pb';
import * as gorm_options_pb from './gorm/options_pb';


export class JournalQueryRequest extends jspb.Message {
  getLookUp(): Query | undefined;
  setLookUp(value?: Query): JournalQueryRequest;
  hasLookUp(): boolean;
  clearLookUp(): JournalQueryRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): JournalQueryRequest.AsObject;
  static toObject(includeInstance: boolean, msg: JournalQueryRequest): JournalQueryRequest.AsObject;
  static serializeBinaryToWriter(message: JournalQueryRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): JournalQueryRequest;
  static deserializeBinaryFromReader(message: JournalQueryRequest, reader: jspb.BinaryReader): JournalQueryRequest;
}

export namespace JournalQueryRequest {
  export type AsObject = {
    lookUp?: Query.AsObject,
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

  getToken(): string;
  setToken(value: string): Handshake;

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
    token: string,
  }
}

export class ProcessDetails extends jspb.Message {
  getProcessId(): string;
  setProcessId(value: string): ProcessDetails;

  getRunningState(): ProcessRunningState;
  setRunningState(value: ProcessRunningState): ProcessDetails;

  getProcessHealth(): ProcessHealthState;
  setProcessHealth(value: ProcessHealthState): ProcessDetails;

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
    processHealth: ProcessHealthState,
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

  getVersion(): string;
  setVersion(value: string): Process;

  getRunningState(): ProcessRunningState;
  setRunningState(value: ProcessRunningState): Process;

  getProcessHealth(): ProcessHealthState;
  setProcessHealth(value: ProcessHealthState): Process;

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
    version: string,
    runningState: ProcessRunningState,
    processHealth: ProcessHealthState,
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

export enum ProcessRunningState { 
  INVALID_PROCESS_RUNNING_STATE = 0,
  PROCESS_STARTING = 1,
  PROCESS_TESTING = 2,
  PROCESS_RUNNING = 3,
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
