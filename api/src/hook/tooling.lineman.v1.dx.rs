mod proto {
    pub use crate::proto::tooling_lineman_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct LinemanServiceServiceHook(proto::lineman_service_client::LinemanServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_lineman_service_service() -> LinemanServiceServiceHook {
    LinemanServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::lineman_service_client::LinemanServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl LinemanServiceServiceHook {
    pub fn create_objective(&self, req: Signal<proto::CreateObjectiveRequest>) -> Resource<Result<proto::Objective, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_objective(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_objective(&self, req: Signal<proto::GetObjectiveRequest>) -> Resource<Result<proto::Objective, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_objective(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_objectives(&self, req: Signal<proto::ListObjectivesRequest>) -> Resource<Result<proto::ListObjectivesResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_objectives(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn create_task(&self, req: Signal<proto::CreateTaskRequest>) -> Resource<Result<proto::Task, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_task(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_task(&self, req: Signal<proto::GetTaskRequest>) -> Resource<Result<proto::Task, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_task(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_tasks(&self, req: Signal<proto::ListTasksRequest>) -> Resource<Result<proto::ListTasksResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_tasks(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn update_task_state(&self, req: Signal<proto::UpdateTaskStateRequest>) -> Resource<Result<proto::Task, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.update_task_state(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn reorder_task(&self, req: Signal<proto::ReorderTaskRequest>) -> Resource<Result<proto::Task, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.reorder_task(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn assign_agent(&self, req: Signal<proto::AssignAgentRequest>) -> Resource<Result<proto::Task, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.assign_agent(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn heartbeat(&self, req: Signal<proto::HeartbeatRequest>) -> Resource<Result<proto::HeartbeatResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.heartbeat(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_agents(&self, req: Signal<proto::ListAgentsRequest>) -> Resource<Result<proto::ListAgentsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_agents(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn request_input(&self, req: Signal<proto::RequestInputRequest>) -> Resource<Result<proto::Task, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.request_input(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn provide_guidance(&self, req: Signal<proto::ProvideGuidanceRequest>) -> Resource<Result<proto::Task, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.provide_guidance(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn create_scheduled_task(&self, req: Signal<proto::CreateScheduledTaskRequest>) -> Resource<Result<proto::ScheduledTask, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_scheduled_task(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_scheduled_tasks(&self, req: Signal<proto::ListScheduledTasksRequest>) -> Resource<Result<proto::ListScheduledTasksResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_scheduled_tasks(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn cancel_scheduled_task(&self, req: Signal<proto::CancelScheduledTaskRequest>) -> Resource<Result<proto::CancelScheduledTaskResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.cancel_scheduled_task(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn create_loop(&self, req: Signal<proto::CreateLoopRequest>) -> Resource<Result<proto::Loop, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_loop(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_loops(&self, req: Signal<proto::ListLoopsRequest>) -> Resource<Result<proto::ListLoopsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_loops(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn pause_loop(&self, req: Signal<proto::PauseLoopRequest>) -> Resource<Result<proto::Loop, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.pause_loop(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn resume_loop(&self, req: Signal<proto::ResumeLoopRequest>) -> Resource<Result<proto::Loop, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.resume_loop(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn delete_loop(&self, req: Signal<proto::DeleteLoopRequest>) -> Resource<Result<proto::DeleteLoopResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.delete_loop(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}