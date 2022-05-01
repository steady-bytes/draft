import * as grpcWeb from 'grpc-web';

import * as eventer_pb from './eventer_pb';


export class EventerClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  emit(
    request: eventer_pb.EmitEventRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: eventer_pb.EmitEventResponse) => void
  ): grpcWeb.ClientReadableStream<eventer_pb.EmitEventResponse>;

}

export class EventerPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  emit(
    request: eventer_pb.EmitEventRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<eventer_pb.EmitEventResponse>;

}

