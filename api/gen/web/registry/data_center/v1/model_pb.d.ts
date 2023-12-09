import * as jspb from 'google-protobuf'



export class DataCenter extends jspb.Message {
  getId(): string;
  setId(value: string): DataCenter;

  getDomain(): string;
  setDomain(value: string): DataCenter;

  getStatus(): DataCenterStatus;
  setStatus(value: DataCenterStatus): DataCenter;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): DataCenter.AsObject;
  static toObject(includeInstance: boolean, msg: DataCenter): DataCenter.AsObject;
  static serializeBinaryToWriter(message: DataCenter, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): DataCenter;
  static deserializeBinaryFromReader(message: DataCenter, reader: jspb.BinaryReader): DataCenter;
}

export namespace DataCenter {
  export type AsObject = {
    id: string,
    domain: string,
    status: DataCenterStatus,
  }
}

export enum DataCenterStatus { 
  UNSPECIFIED_DATA_CENTER_STATUS = 0,
  DATA_CENTER_REGISTERED = 1,
  DATA_CENTER_ONLINE = 2,
  DATA_CENTER_OFFLINE = 3,
  DATA_CENTER_DEREGISTERED = 4,
}
