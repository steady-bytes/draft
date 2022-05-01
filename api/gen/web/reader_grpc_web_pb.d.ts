import * as grpcWeb from 'grpc-web';

import * as reader_pb from './reader_pb';


export class ReaderClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  read(
    request: reader_pb.ReadRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: reader_pb.ReadResponse) => void
  ): grpcWeb.ClientReadableStream<reader_pb.ReadResponse>;

}

export class ReaderPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  read(
    request: reader_pb.ReadRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<reader_pb.ReadResponse>;

}

