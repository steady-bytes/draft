import * as jspb from 'google-protobuf'

import * as google_protobuf_any_pb from 'google-protobuf/google/protobuf/any_pb';
import * as google_protobuf_timestamp_pb from 'google-protobuf/google/protobuf/timestamp_pb';
import * as validate_validate_pb from './validate/validate_pb';


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

