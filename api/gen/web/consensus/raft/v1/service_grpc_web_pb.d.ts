import * as grpcWeb from 'grpc-web';

import * as consensus_raft_v1_service_pb from '../../../consensus/raft/v1/service_pb'; // proto import: "consensus/raft/v1/service.proto"


export class RaftServiceClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  join(
    request: consensus_raft_v1_service_pb.JoinRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: consensus_raft_v1_service_pb.JoinResponse) => void
  ): grpcWeb.ClientReadableStream<consensus_raft_v1_service_pb.JoinResponse>;

  remove(
    request: consensus_raft_v1_service_pb.RemoveRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: consensus_raft_v1_service_pb.RemoveResponse) => void
  ): grpcWeb.ClientReadableStream<consensus_raft_v1_service_pb.RemoveResponse>;

  stats(
    request: consensus_raft_v1_service_pb.StatsRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: consensus_raft_v1_service_pb.StatsResponse) => void
  ): grpcWeb.ClientReadableStream<consensus_raft_v1_service_pb.StatsResponse>;

}

export class RaftServicePromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  join(
    request: consensus_raft_v1_service_pb.JoinRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<consensus_raft_v1_service_pb.JoinResponse>;

  remove(
    request: consensus_raft_v1_service_pb.RemoveRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<consensus_raft_v1_service_pb.RemoveResponse>;

  stats(
    request: consensus_raft_v1_service_pb.StatsRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<consensus_raft_v1_service_pb.StatsResponse>;

}

