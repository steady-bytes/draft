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

export class RegistryClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  join(
    request: draft_pb.JoinRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: draft_pb.JoinResponse) => void
  ): grpcWeb.ClientReadableStream<draft_pb.JoinResponse>;

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

export class RegistryPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  join(
    request: draft_pb.JoinRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<draft_pb.JoinResponse>;

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

