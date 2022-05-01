import * as grpcWeb from 'grpc-web';

import * as registrar_pb from './registrar_pb';


export class RegistryClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  initiateHandshake(
    request: registrar_pb.RequestHandshake,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registrar_pb.Handshake) => void
  ): grpcWeb.ClientReadableStream<registrar_pb.Handshake>;

  disconnect(
    request: registrar_pb.DisconnectRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registrar_pb.Disconnected) => void
  ): grpcWeb.ClientReadableStream<registrar_pb.Disconnected>;

  monitor(
    request: registrar_pb.MonitorRequest,
    metadata?: grpcWeb.Metadata
  ): grpcWeb.ClientReadableStream<registrar_pb.Process>;

  querySystemJournal(
    request: registrar_pb.JournalQueryRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registrar_pb.JournalQueryResponse) => void
  ): grpcWeb.ClientReadableStream<registrar_pb.JournalQueryResponse>;

}

export class RegistryPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  initiateHandshake(
    request: registrar_pb.RequestHandshake,
    metadata?: grpcWeb.Metadata
  ): Promise<registrar_pb.Handshake>;

  disconnect(
    request: registrar_pb.DisconnectRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registrar_pb.Disconnected>;

  monitor(
    request: registrar_pb.MonitorRequest,
    metadata?: grpcWeb.Metadata
  ): grpcWeb.ClientReadableStream<registrar_pb.Process>;

  querySystemJournal(
    request: registrar_pb.JournalQueryRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registrar_pb.JournalQueryResponse>;

}

