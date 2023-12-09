import * as grpcWeb from 'grpc-web';

import * as test_hello_world_v1_service_pb from '../../../test/hello_world/v1/service_pb'; // proto import: "test/hello_world/v1/service.proto"


export class HelloWorldClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  hello(
    request: test_hello_world_v1_service_pb.HelloRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: test_hello_world_v1_service_pb.HelloResponse) => void
  ): grpcWeb.ClientReadableStream<test_hello_world_v1_service_pb.HelloResponse>;

}

export class HelloWorldPromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  hello(
    request: test_hello_world_v1_service_pb.HelloRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<test_hello_world_v1_service_pb.HelloResponse>;

}

