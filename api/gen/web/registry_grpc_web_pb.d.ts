import * as grpcWeb from 'grpc-web';

import * as registry_pb from './registry_pb';


export class RegistryClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  initiateHandshake(
    request: registry_pb.RequestHandshake,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_pb.Handshake) => void
  ): grpcWeb.ClientReadableStream<registry_pb.Handshake>;

  disconnect(
    request: registry_pb.DisconnectRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_pb.Disconnected) => void
  ): grpcWeb.ClientReadableStream<registry_pb.Disconnected>;

  monitor(
    request: registry_pb.MonitorRequest,
    metadata?: grpcWeb.Metadata
  ): grpcWeb.ClientReadableStream<registry_pb.Process>;

  querySystemJournal(
    request: registry_pb.JournalQueryRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_pb.JournalQueryResponse) => void
  ): grpcWeb.ClientReadableStream<registry_pb.JournalQueryResponse>;

}

export class RegistryPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  initiateHandshake(
    request: registry_pb.RequestHandshake,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_pb.Handshake>;

  disconnect(
    request: registry_pb.DisconnectRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_pb.Disconnected>;

  monitor(
    request: registry_pb.MonitorRequest,
    metadata?: grpcWeb.Metadata
  ): grpcWeb.ClientReadableStream<registry_pb.Process>;

  querySystemJournal(
    request: registry_pb.JournalQueryRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_pb.JournalQueryResponse>;

}

