import * as grpcWeb from 'grpc-web';

import * as draft_pb from './draft_pb';


export class WriterClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  exec(
    request: draft_pb.Command,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: draft_pb.Output) => void
  ): grpcWeb.ClientReadableStream<draft_pb.Output>;

  execSaga(
    request: draft_pb.Command,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: draft_pb.Transaction) => void
  ): grpcWeb.ClientReadableStream<draft_pb.Transaction>;

}

export class EventStoreClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  create(
    request: draft_pb.CreateEventRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: draft_pb.CreateEventResponse) => void
  ): grpcWeb.ClientReadableStream<draft_pb.CreateEventResponse>;

}

export class RegistryClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  initiateHandshake(
    request: draft_pb.RequestHandshake,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: draft_pb.Handshake) => void
  ): grpcWeb.ClientReadableStream<draft_pb.Handshake>;

  disconnect(
    request: draft_pb.DisconnectRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: draft_pb.Disconnected) => void
  ): grpcWeb.ClientReadableStream<draft_pb.Disconnected>;

  monitor(
    request: draft_pb.MonitorRequest,
    metadata?: grpcWeb.Metadata
  ): grpcWeb.ClientReadableStream<draft_pb.Process>;

  querySystemJournal(
    request: draft_pb.JournalQueryRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: draft_pb.JournalQueryResponse) => void
  ): grpcWeb.ClientReadableStream<draft_pb.JournalQueryResponse>;

}

export class WriterPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  exec(
    request: draft_pb.Command,
    metadata?: grpcWeb.Metadata
  ): Promise<draft_pb.Output>;

  execSaga(
    request: draft_pb.Command,
    metadata?: grpcWeb.Metadata
  ): Promise<draft_pb.Transaction>;

}

export class EventStorePromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  create(
    request: draft_pb.CreateEventRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<draft_pb.CreateEventResponse>;

}

export class RegistryPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  initiateHandshake(
    request: draft_pb.RequestHandshake,
    metadata?: grpcWeb.Metadata
  ): Promise<draft_pb.Handshake>;

  disconnect(
    request: draft_pb.DisconnectRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<draft_pb.Disconnected>;

  monitor(
    request: draft_pb.MonitorRequest,
    metadata?: grpcWeb.Metadata
  ): grpcWeb.ClientReadableStream<draft_pb.Process>;

  querySystemJournal(
    request: draft_pb.JournalQueryRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<draft_pb.JournalQueryResponse>;

}

