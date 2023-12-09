import * as jspb from 'google-protobuf'

import * as registry_data_center_v1_model_pb from '../../../registry/data_center/v1/model_pb'; // proto import: "registry/data_center/v1/model.proto"


export class RegisterRequest extends jspb.Message {
  getName(): string;
  setName(value: string): RegisterRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): RegisterRequest.AsObject;
  static toObject(includeInstance: boolean, msg: RegisterRequest): RegisterRequest.AsObject;
  static serializeBinaryToWriter(message: RegisterRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): RegisterRequest;
  static deserializeBinaryFromReader(message: RegisterRequest, reader: jspb.BinaryReader): RegisterRequest;
}

export namespace RegisterRequest {
  export type AsObject = {
    name: string,
  }
}

export class RegisterResponse extends jspb.Message {
  getDataCenterId(): string;
  setDataCenterId(value: string): RegisterResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): RegisterResponse.AsObject;
  static toObject(includeInstance: boolean, msg: RegisterResponse): RegisterResponse.AsObject;
  static serializeBinaryToWriter(message: RegisterResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): RegisterResponse;
  static deserializeBinaryFromReader(message: RegisterResponse, reader: jspb.BinaryReader): RegisterResponse;
}

export namespace RegisterResponse {
  export type AsObject = {
    dataCenterId: string,
  }
}

export class DeregisterRequest extends jspb.Message {
  getDataCenterId(): string;
  setDataCenterId(value: string): DeregisterRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): DeregisterRequest.AsObject;
  static toObject(includeInstance: boolean, msg: DeregisterRequest): DeregisterRequest.AsObject;
  static serializeBinaryToWriter(message: DeregisterRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): DeregisterRequest;
  static deserializeBinaryFromReader(message: DeregisterRequest, reader: jspb.BinaryReader): DeregisterRequest;
}

export namespace DeregisterRequest {
  export type AsObject = {
    dataCenterId: string,
  }
}

export class DeregisterResponse extends jspb.Message {
  getDataCenterId(): string;
  setDataCenterId(value: string): DeregisterResponse;

  getStatus(): registry_data_center_v1_model_pb.DataCenterStatus;
  setStatus(value: registry_data_center_v1_model_pb.DataCenterStatus): DeregisterResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): DeregisterResponse.AsObject;
  static toObject(includeInstance: boolean, msg: DeregisterResponse): DeregisterResponse.AsObject;
  static serializeBinaryToWriter(message: DeregisterResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): DeregisterResponse;
  static deserializeBinaryFromReader(message: DeregisterResponse, reader: jspb.BinaryReader): DeregisterResponse;
}

export namespace DeregisterResponse {
  export type AsObject = {
    dataCenterId: string,
    status: registry_data_center_v1_model_pb.DataCenterStatus,
  }
}

export class QueryRequest extends jspb.Message {
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): QueryRequest.AsObject;
  static toObject(includeInstance: boolean, msg: QueryRequest): QueryRequest.AsObject;
  static serializeBinaryToWriter(message: QueryRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): QueryRequest;
  static deserializeBinaryFromReader(message: QueryRequest, reader: jspb.BinaryReader): QueryRequest;
}

export namespace QueryRequest {
  export type AsObject = {
  }
}

export class QueryResponse extends jspb.Message {
  getDataCentersList(): Array<registry_data_center_v1_model_pb.DataCenter>;
  setDataCentersList(value: Array<registry_data_center_v1_model_pb.DataCenter>): QueryResponse;
  clearDataCentersList(): QueryResponse;
  addDataCenters(value?: registry_data_center_v1_model_pb.DataCenter, index?: number): registry_data_center_v1_model_pb.DataCenter;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): QueryResponse.AsObject;
  static toObject(includeInstance: boolean, msg: QueryResponse): QueryResponse.AsObject;
  static serializeBinaryToWriter(message: QueryResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): QueryResponse;
  static deserializeBinaryFromReader(message: QueryResponse, reader: jspb.BinaryReader): QueryResponse;
}

export namespace QueryResponse {
  export type AsObject = {
    dataCentersList: Array<registry_data_center_v1_model_pb.DataCenter.AsObject>,
  }
}

