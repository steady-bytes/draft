import * as grpcWeb from 'grpc-web';

import * as writer_pb from './writer_pb';


export class WriterClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  exec(
    request: writer_pb.Command,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: writer_pb.Output) => void
  ): grpcWeb.ClientReadableStream<writer_pb.Output>;

  execSaga(
    request: writer_pb.Command,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: writer_pb.Transaction) => void
  ): grpcWeb.ClientReadableStream<writer_pb.Transaction>;

}

export class WriterPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  exec(
    request: writer_pb.Command,
    metadata?: grpcWeb.Metadata
  ): Promise<writer_pb.Output>;

  execSaga(
    request: writer_pb.Command,
    metadata?: grpcWeb.Metadata
  ): Promise<writer_pb.Transaction>;

}

