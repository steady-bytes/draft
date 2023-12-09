import * as grpcWeb from 'grpc-web';

import * as registry_service_discovery_v1_service_pb from '../../../registry/service_discovery/v1/service_pb'; // proto import: "registry/service_discovery/v1/service.proto"


export class ServiceDiscoveryServiceClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  init(
    request: registry_service_discovery_v1_service_pb.InitRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_service_discovery_v1_service_pb.InitResponse) => void
  ): grpcWeb.ClientReadableStream<registry_service_discovery_v1_service_pb.InitResponse>;

  querySystemJournal(
    request: registry_service_discovery_v1_service_pb.JournalQueryRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: registry_service_discovery_v1_service_pb.JournalQueryResponse) => void
  ): grpcWeb.ClientReadableStream<registry_service_discovery_v1_service_pb.JournalQueryResponse>;

}

export class ServiceDiscoveryServicePromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  init(
    request: registry_service_discovery_v1_service_pb.InitRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_service_discovery_v1_service_pb.InitResponse>;

  querySystemJournal(
    request: registry_service_discovery_v1_service_pb.JournalQueryRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<registry_service_discovery_v1_service_pb.JournalQueryResponse>;

}

