import * as jspb from 'google-protobuf'



export class UUIDValue extends jspb.Message {
  getValue(): string;
  setValue(value: string): UUIDValue;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): UUIDValue.AsObject;
  static toObject(includeInstance: boolean, msg: UUIDValue): UUIDValue.AsObject;
  static serializeBinaryToWriter(message: UUIDValue, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): UUIDValue;
  static deserializeBinaryFromReader(message: UUIDValue, reader: jspb.BinaryReader): UUIDValue;
}

export namespace UUIDValue {
  export type AsObject = {
    value: string,
  }
}

export class JSONValue extends jspb.Message {
  getValue(): string;
  setValue(value: string): JSONValue;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): JSONValue.AsObject;
  static toObject(includeInstance: boolean, msg: JSONValue): JSONValue.AsObject;
  static serializeBinaryToWriter(message: JSONValue, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): JSONValue;
  static deserializeBinaryFromReader(message: JSONValue, reader: jspb.BinaryReader): JSONValue;
}

export namespace JSONValue {
  export type AsObject = {
    value: string,
  }
}

export class UUID extends jspb.Message {
  getValue(): string;
  setValue(value: string): UUID;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): UUID.AsObject;
  static toObject(includeInstance: boolean, msg: UUID): UUID.AsObject;
  static serializeBinaryToWriter(message: UUID, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): UUID;
  static deserializeBinaryFromReader(message: UUID, reader: jspb.BinaryReader): UUID;
}

export namespace UUID {
  export type AsObject = {
    value: string,
  }
}

export class InetValue extends jspb.Message {
  getValue(): string;
  setValue(value: string): InetValue;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): InetValue.AsObject;
  static toObject(includeInstance: boolean, msg: InetValue): InetValue.AsObject;
  static serializeBinaryToWriter(message: InetValue, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): InetValue;
  static deserializeBinaryFromReader(message: InetValue, reader: jspb.BinaryReader): InetValue;
}

export namespace InetValue {
  export type AsObject = {
    value: string,
  }
}

export class TimeOnly extends jspb.Message {
  getValue(): number;
  setValue(value: number): TimeOnly;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): TimeOnly.AsObject;
  static toObject(includeInstance: boolean, msg: TimeOnly): TimeOnly.AsObject;
  static serializeBinaryToWriter(message: TimeOnly, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): TimeOnly;
  static deserializeBinaryFromReader(message: TimeOnly, reader: jspb.BinaryReader): TimeOnly;
}

export namespace TimeOnly {
  export type AsObject = {
    value: number,
  }
}

export class BigInt extends jspb.Message {
  getValue(): string;
  setValue(value: string): BigInt;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): BigInt.AsObject;
  static toObject(includeInstance: boolean, msg: BigInt): BigInt.AsObject;
  static serializeBinaryToWriter(message: BigInt, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): BigInt;
  static deserializeBinaryFromReader(message: BigInt, reader: jspb.BinaryReader): BigInt;
}

export namespace BigInt {
  export type AsObject = {
    value: string,
  }
}

