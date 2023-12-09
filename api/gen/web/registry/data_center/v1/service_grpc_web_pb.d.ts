import * as grpcWeb from 'grpc-web';

import * as registry_data_center_v1_service_pb from '../../../registry/data_center/v1/service_pb'; // proto import: "registry/data_center/v1/service.proto"


export class DataCenterServiceClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  register(
    request: registry_data_center_v1_service_pb.RegisterRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_data_center_v1_service_pb.RegisterResponse) => void
  ): grpcWeb.ClientReadableStream<registry_data_center_v1_service_pb.RegisterResponse>;

  deregister(
    request: registry_data_center_v1_service_pb.DeregisterRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_data_center_v1_service_pb.DeregisterResponse) => void
  ): grpcWeb.ClientReadableStream<registry_data_center_v1_service_pb.DeregisterResponse>;

  query(
    request: registry_data_center_v1_service_pb.QueryRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_data_center_v1_service_pb.QueryResponse) => void
  ): grpcWeb.ClientReadableStream<registry_data_center_v1_service_pb.QueryResponse>;

}

export class DataCenterServicePromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  register(
    request: registry_data_center_v1_service_pb.RegisterRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_data_center_v1_service_pb.RegisterResponse>;

  deregister(
    request: registry_data_center_v1_service_pb.DeregisterRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_data_center_v1_service_pb.DeregisterResponse>;

  query(
    request: registry_data_center_v1_service_pb.QueryRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_data_center_v1_service_pb.QueryResponse>;

}

