mod proto {
    pub use crate::proto::tooling_allele_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct AlleleServiceServiceHook(proto::allele_service_client::AlleleServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_allele_service_service() -> AlleleServiceServiceHook {
    AlleleServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::allele_service_client::AlleleServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl AlleleServiceServiceHook {
    pub fn list_repositories(&self, req: Signal<proto::ListRepositoriesRequest>) -> Resource<Result<proto::ListRepositoriesResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_repositories(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn create_repository(&self, req: Signal<proto::CreateRepositoryRequest>) -> Resource<Result<proto::Repository, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_repository(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_repository(&self, req: Signal<proto::GetRepositoryRequest>) -> Resource<Result<proto::Repository, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_repository(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_worktrees(&self, req: Signal<proto::ListWorktreesRequest>) -> Resource<Result<proto::ListWorktreesResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_worktrees(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_worktree_overlap(&self, req: Signal<proto::GetWorktreeOverlapRequest>) -> Resource<Result<proto::GetWorktreeOverlapResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_worktree_overlap(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn close_worktree(&self, req: Signal<proto::CloseWorktreeRequest>) -> Resource<Result<proto::CloseWorktreeResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.close_worktree(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_changes(&self, req: Signal<proto::ListChangesRequest>) -> Resource<Result<proto::ListChangesResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_changes(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_change(&self, req: Signal<proto::GetChangeRequest>) -> Resource<Result<proto::Change, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_change(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn rebase_change(&self, req: Signal<proto::RebaseChangeRequest>) -> Resource<Result<proto::Change, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.rebase_change(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn enqueue_merge(&self, req: Signal<proto::EnqueueMergeRequest>) -> Resource<Result<proto::MergeQueueEntry, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.enqueue_merge(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_merge_queue(&self, req: Signal<proto::ListMergeQueueRequest>) -> Resource<Result<proto::ListMergeQueueResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_merge_queue(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn hold_merge_queue_entry(&self, req: Signal<proto::HoldMergeQueueEntryRequest>) -> Resource<Result<proto::MergeQueueEntry, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.hold_merge_queue_entry(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn resume_merge_queue_entry(&self, req: Signal<proto::ResumeMergeQueueEntryRequest>) -> Resource<Result<proto::MergeQueueEntry, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.resume_merge_queue_entry(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn notify_push(&self, req: Signal<proto::NotifyPushRequest>) -> Resource<Result<proto::NotifyPushResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.notify_push(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_provenance(&self, req: Signal<proto::GetProvenanceRequest>) -> Resource<Result<proto::Provenance, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_provenance(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_path_permissions(&self, req: Signal<proto::ListPathPermissionsRequest>) -> Resource<Result<proto::ListPathPermissionsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_path_permissions(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn set_path_permissions(&self, req: Signal<proto::SetPathPermissionsRequest>) -> Resource<Result<proto::SetPathPermissionsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.set_path_permissions(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn check_push_permission(&self, req: Signal<proto::CheckPushPermissionRequest>) -> Resource<Result<proto::CheckPushPermissionResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.check_push_permission(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn register_signing_key(&self, req: Signal<proto::RegisterSigningKeyRequest>) -> Resource<Result<proto::SigningKey, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.register_signing_key(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_signing_keys(&self, req: Signal<proto::ListSigningKeysRequest>) -> Resource<Result<proto::ListSigningKeysResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_signing_keys(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn revoke_signing_key(&self, req: Signal<proto::RevokeSigningKeyRequest>) -> Resource<Result<proto::RevokeSigningKeyResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.revoke_signing_key(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_linkage_manifest(&self, req: Signal<proto::GetLinkageManifestRequest>) -> Resource<Result<proto::LinkageManifest, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_linkage_manifest(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn diff_linkage_manifest(&self, req: Signal<proto::DiffLinkageManifestRequest>) -> Resource<Result<proto::DiffLinkageManifestResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.diff_linkage_manifest(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn set_breaking_change_policy(&self, req: Signal<proto::SetBreakingChangePolicyRequest>) -> Resource<Result<proto::LinkageManifest, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.set_breaking_change_policy(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn approve_linkage_change(&self, req: Signal<proto::ApproveLinkageChangeRequest>) -> Resource<Result<proto::ApproveLinkageChangeResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.approve_linkage_change(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}