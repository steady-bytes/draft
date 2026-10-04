mod proto {
    pub use crate::proto::tooling_relay_v1::*;
}
pub use proto::*;
use ::dioxus::prelude::*;

pub struct RelayServiceServiceHook(proto::relay_service_client::RelayServiceClient<::tonic_web_wasm_client::Client>);

pub fn use_relay_service_service() -> RelayServiceServiceHook {
    RelayServiceServiceHook({ let config = use_context::<::dioxus_grpc::GrpcConfig>(); proto::relay_service_client::RelayServiceClient::new(::tonic_web_wasm_client::Client::new(config.host.clone())) })
}

impl RelayServiceServiceHook {
    pub fn list_devices(&self, req: Signal<proto::ListDevicesRequest>) -> Resource<Result<proto::ListDevicesResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_devices(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_tracks(&self, req: Signal<proto::ListTracksRequest>) -> Resource<Result<proto::ListTracksResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_tracks(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn add_track(&self, req: Signal<proto::AddTrackRequest>) -> Resource<Result<proto::Track, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.add_track(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn rescan_library(&self, req: Signal<proto::RescanLibraryRequest>) -> Resource<Result<proto::RescanLibraryResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.rescan_library(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn delete_track(&self, req: Signal<proto::DeleteTrackRequest>) -> Resource<Result<proto::DeleteTrackResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.delete_track(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn play(&self, req: Signal<proto::PlayRequest>) -> Resource<Result<proto::PlayerState, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.play(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn pause(&self, req: Signal<proto::PauseRequest>) -> Resource<Result<proto::PlayerState, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.pause(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn seek(&self, req: Signal<proto::SeekRequest>) -> Resource<Result<proto::PlayerState, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.seek(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn set_volume(&self, req: Signal<proto::SetVolumeRequest>) -> Resource<Result<proto::PlayerState, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.set_volume(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_player_state(&self, req: Signal<proto::GetPlayerStateRequest>) -> Resource<Result<proto::PlayerState, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_player_state(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn enqueue(&self, req: Signal<proto::EnqueueRequest>) -> Resource<Result<proto::QueueState, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.enqueue(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn clear_queue(&self, req: Signal<proto::ClearQueueRequest>) -> Resource<Result<proto::QueueState, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.clear_queue(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn start_recording(&self, req: Signal<proto::StartRecordingRequest>) -> Resource<Result<proto::Recording, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.start_recording(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn stream_audio_in_chunk(&self, req: Signal<proto::AudioChunk>) -> Resource<Result<proto::StreamAudioInChunkResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.stream_audio_in_chunk(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn add_marker(&self, req: Signal<proto::AddMarkerRequest>) -> Resource<Result<proto::Marker, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.add_marker(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn stop_recording(&self, req: Signal<proto::StopRecordingRequest>) -> Resource<Result<proto::Recording, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.stop_recording(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn discard_recording(&self, req: Signal<proto::DiscardRecordingRequest>) -> Resource<Result<proto::DiscardRecordingResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.discard_recording(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_recordings(&self, req: Signal<proto::ListRecordingsRequest>) -> Resource<Result<proto::ListRecordingsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_recordings(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_recording(&self, req: Signal<proto::GetRecordingRequest>) -> Resource<Result<proto::Recording, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_recording(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn get_transcript(&self, req: Signal<proto::GetTranscriptRequest>) -> Resource<Result<proto::GetTranscriptResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.get_transcript(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn search_transcripts(&self, req: Signal<proto::SearchTranscriptsRequest>) -> Resource<Result<proto::SearchTranscriptsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.search_transcripts(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn rename_speaker(&self, req: Signal<proto::RenameSpeakerRequest>) -> Resource<Result<proto::Speaker, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.rename_speaker(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn delete_recording(&self, req: Signal<proto::DeleteRecordingRequest>) -> Resource<Result<proto::DeleteRecordingResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.delete_recording(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn create_action_item(&self, req: Signal<proto::CreateActionItemRequest>) -> Resource<Result<proto::ActionItem, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_action_item(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn list_action_items(&self, req: Signal<proto::ListActionItemsRequest>) -> Resource<Result<proto::ListActionItemsResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.list_action_items(req()).await.map(|resp| resp.into_inner()) }
        })
    }
    pub fn create_lineman_tasks(&self, req: Signal<proto::CreateLinemanTasksRequest>) -> Resource<Result<proto::CreateLinemanTasksResponse, tonic::Status>> {
        let client = self.0.to_owned();
        use_resource(move || {
            let mut client = client.clone();
            async move { client.create_lineman_tasks(req()).await.map(|resp| resp.into_inner()) }
        })
    }
}