//! Library: browse/search/upload tracks, play one back through the browser's own speakers.
//!
//! Real playback, no mock waveform: `StreamAudioOut`'s raw PCM16LE chunks are scheduled through a
//! real Web Audio graph (assets/audio-player.js), not a canvas animation. A plain position bar
//! stands in for the mockup's own peak-data waveform -- see the implementation plan's Phase 10
//! completion note for that disclosed scope trim. The "Output" device is a fixed label, not a
//! dropdown: browser playback only ever targets this browser's own speakers (`browser-out`); a
//! dropdown offering the mockup's other example devices ("Studio monitors", "Office speakers")
//! would be exactly the fake/mock UI this phase's own deliverable bar rules out, since Play/
//! StartRecording-to-a-real-native-device routing is Phase 9's disclosed, not-yet-built remainder.

use base64::{engine::general_purpose::STANDARD, Engine};
use dioxus::prelude::*;
use draft_api::proto::tooling_relay_v1::{
    AddTrackRequest, DeleteTrackRequest, GetPlayerStateRequest, ListTracksRequest, PauseRequest,
    PlayRequest, PlayerState, SeekRequest, SetVolumeRequest, StreamAudioOutRequest, Track,
    UploadTrackChunk,
};
use draft_ui::layout::{Drawer, PageHead, Split};
use draft_ui::ui::{use_toast, Btn, BtnVariant, Empty, Loading, Modal, TextInput, Toast};
use gloo_timers::future::TimeoutFuture;

use crate::api::client;
use crate::components::{format_bytes, format_duration_ms};

/// Every Play/Pause/Seek/SetVolume/GetPlayerState/StreamAudioOut call in this view targets this
/// one output device -- see this file's own doc comment for why there's no picker.
const OUTPUT_DEVICE_ID: &str = "browser-out";

/// Fallback PCM format for a track whose sample_rate_hz/channels came back 0 (an AddTrack-only
/// metadata row with no uploaded file yet). Matches whisper.cpp's own recording format.
const FALLBACK_SAMPLE_RATE: i32 = 16_000;
const FALLBACK_CHANNELS: i32 = 1;

#[component]
pub fn Library() -> Element {
    let mut search = use_signal(String::new);
    let mut tracks_res = use_resource(move || {
        let filter = search();
        async move {
            client()
                .list_tracks(ListTracksRequest { filter })
                .await
                .map(|r| r.into_inner().tracks)
                .unwrap_or_default()
        }
    });

    let mut player_state = use_signal(PlayerState::default);
    // What the drawer shows -- separate from player_state.track_id so clicking a row updates the
    // drawer immediately, without waiting on Play's own round trip.
    let mut selected: Signal<Option<Track>> = use_signal(|| None);
    // Set while the "Delete this track?" confirmation is up -- same Modal-backed shape
    // Blueprint's own Gateway page uses for DeleteRoute (services/core/blueprint/web-client/src/
    // views/gateway.rs), not a native `confirm()` (blocks the whole tab, can't be styled).
    let mut confirm_delete: Signal<Option<Track>> = use_signal(|| None);
    let toast = use_toast();

    // Poll GetPlayerState while mounted: authoritative position/playing/volume for the shared
    // browser output device -- another tab could be driving it (the plan's own "one PlayerState
    // per output device, not per listener" decision).
    use_coroutine(move |_: UnboundedReceiver<()>| async move {
        loop {
            if let Ok(resp) = client()
                .get_player_state(GetPlayerStateRequest { output_device_id: OUTPUT_DEVICE_ID.to_string() })
                .await
            {
                player_state.set(resp.into_inner());
            }
            TimeoutFuture::new(750).await;
        }
    });

    // The one persistent StreamAudioOut receive loop for this page's lifetime. The server decides
    // what to actually send (nothing while paused -- rpc.go's StreamAudioOut blocks on Wake());
    // this loop just forwards whatever real bytes arrive to the real Web Audio player. on_play
    // (below) calls window.relayAudio.start(...) with the clicked track's own format at the moment
    // playback begins -- this loop never needs to know a track changed to stay correct.
    use_coroutine(move |_: UnboundedReceiver<()>| async move {
        let Ok(resp) = client()
            .stream_audio_out(StreamAudioOutRequest { output_device_id: OUTPUT_DEVICE_ID.to_string() })
            .await
        else {
            return;
        };
        let mut stream = resp.into_inner();
        while let Ok(Some(chunk)) = stream.message().await {
            if chunk.pcm.is_empty() {
                continue;
            }
            let b64 = STANDARD.encode(&chunk.pcm);
            document::eval(&format!("window.relayAudio.pushChunk('{b64}')"));
        }
    });

    let state = player_state();
    let is_playing = state.playing;

    let on_play = use_callback(move |track: Track| {
        let rate = if track.sample_rate_hz > 0 { track.sample_rate_hz } else { FALLBACK_SAMPLE_RATE };
        let channels = if track.channels > 0 { track.channels } else { FALLBACK_CHANNELS };
        let volume = player_state().volume_pct.max(1);
        document::eval(&format!("window.relayAudio.start({rate}, {channels}, {volume})"));
        selected.set(Some(track.clone()));
        spawn(async move {
            let _ = client()
                .play(PlayRequest { track_id: track.id, output_device_id: OUTPUT_DEVICE_ID.to_string() })
                .await;
        });
    });

    let on_pause = move |_: ()| {
        spawn(async move {
            let _ = client().pause(PauseRequest { output_device_id: OUTPUT_DEVICE_ID.to_string() }).await;
        });
    };

    let on_seek = move |position_ms: i64| {
        spawn(async move {
            let _ = client()
                .seek(SeekRequest { output_device_id: OUTPUT_DEVICE_ID.to_string(), position_ms })
                .await;
        });
    };

    let on_volume = move |pct: i32| {
        document::eval(&format!("window.relayAudio.setVolume({pct})"));
        spawn(async move {
            let _ = client()
                .set_volume(SetVolumeRequest { output_device_id: OUTPUT_DEVICE_ID.to_string(), volume_pct: pct })
                .await;
        });
    };

    // Deletes whatever track is in `confirm_delete`, not the row that happened to open the
    // drawer -- the two can be different tracks since the Modal stays up across any player state
    // changes (it reads its own signal, not `selected`).
    let delete = use_callback(move |track: Track| {
        spawn(async move {
            match client().delete_track(DeleteTrackRequest { track_id: track.id.clone() }).await {
                Ok(_) => {
                    confirm_delete.set(None);
                    if selected.peek().as_ref().map(|t| t.id.as_str()) == Some(track.id.as_str()) {
                        selected.set(None);
                    }
                    tracks_res.restart();
                    toast.show(format!("Deleted {}", display_title(&track)), 3000);
                }
                Err(e) => {
                    confirm_delete.set(None);
                    toast.show(format!("Could not delete: {}", e.message()), 6000);
                }
            }
        });
    });

    let drawer = selected().map(|track| {
        let is_current = state.track_id == track.id;
        let track_for_delete = track.clone();
        rsx! {
            NowPlaying {
                track: track.clone(),
                state: state.clone(),
                is_playing: is_current && is_playing,
                on_play,
                on_pause,
                on_seek,
                on_volume,
                on_close: move |_| selected.set(None),
                on_delete: move |_| confirm_delete.set(Some(track_for_delete.clone())),
            }
        }
    });

    rsx! {
        document::Script { src: asset!("/assets/audio-player.js") }

        PageHead {
            title: "Library".to_string(),
            eyebrow: "Relay · stored audio".to_string(),
            description: "Music and audio files Relay can play back through the browser.".to_string(),
            actions: rsx! { UploadButton { on_uploaded: move |_| tracks_res.restart() } },
        }

        div { style: "margin: 4px 0 16px",
            TextInput {
                value: search(),
                oninput: move |v| search.set(v),
                placeholder: "Search title, artist, album…".to_string(),
                aria_label: "Search library".to_string(),
            }
        }

        match &*tracks_res.read() {
            None => rsx! { Loading {} },
            Some(tracks) if tracks.is_empty() => rsx! {
                Empty { title: "No tracks yet".to_string(), "Add a WAV file to get started." }
            },
            Some(tracks) => rsx! {
                Split { drawer,
                    // Fills down to the bottom of the viewport instead of shrinking to content
                    // height (leaving a dead gap below a short list) -- 300px covers everything
                    // above this panel that's effectively constant regardless of viewport size
                    // (topbar, .d-main's own padding, PageHead, the search bar) plus .d-main's own
                    // bottom padding, measured live via the actual rendered layout. `d-table-wrap`
                    // already carries `overflow:auto` (components/_table.scss), so this only adds
                    // the fixed height that overflow needs something to actually trigger against.
                    div { class: "d-panel d-table-wrap", style: "height: calc(100vh - 300px)",
                        table { class: "d-table",
                            thead {
                                tr { th { "Title" } th { "Artist" } th { "Album" } th { "Format" } th { class: "is-right", "Length" } }
                            }
                            tbody {
                                for track in tracks.iter().cloned() {
                                    TrackRow {
                                        key: "{track.id}",
                                        track: track.clone(),
                                        is_current: state.track_id == track.id,
                                        is_playing: state.track_id == track.id && is_playing,
                                        onclick: move |_| on_play(track.clone()),
                                    }
                                }
                            }
                        }
                    }
                }
            },
        }

        Modal {
            open: confirm_delete().is_some(),
            title: "Delete track?".to_string(),
            on_close: move |_| confirm_delete.set(None),
            footer: rsx! {
                Btn { variant: BtnVariant::Ghost, onclick: move |_| confirm_delete.set(None), "Cancel" }
                Btn {
                    variant: BtnVariant::Danger,
                    onclick: move |_| {
                        if let Some(track) = confirm_delete.peek().clone() {
                            delete.call(track);
                        }
                    },
                    "Delete"
                }
            },
            if let Some(track) = confirm_delete() {
                p { style: "margin:0",
                    "Removes " b { "{display_title(&track)}" } " and its audio file. This can't be undone."
                }
            }
        }
        Toast { state: toast }
    }
}

/// "Untitled" for a track with no title -- a recording whose own name (Record page's Name field)
/// didn't make it to this track for some reason, or a plain library upload nobody named. Shared by
/// `TrackRow` and the delete confirmation/toast so the two can never show a different label for the
/// same track.
fn display_title(track: &Track) -> String {
    if track.title.is_empty() { "Untitled".to_string() } else { track.title.clone() }
}

#[component]
fn TrackRow(track: Track, is_current: bool, is_playing: bool, onclick: EventHandler<()>) -> Element {
    let title = display_title(&track);
    rsx! {
        tr {
            aria_selected: if is_current { "true" } else { "false" },
            style: "cursor:pointer",
            onclick: move |_| onclick.call(()),
            td {
                if is_current && is_playing {
                    span { style: "color:var(--primary); margin-right:6px", "▶" }
                }
                b { "{title}" }
            }
            td { "{track.artist}" }
            td { "{track.album}" }
            td { "{track.format}" }
            td { class: "is-right", "{format_duration_ms(track.duration_ms)}" }
        }
    }
}

#[component]
fn UploadButton(on_uploaded: EventHandler<()>) -> Element {
    let mut uploading = use_signal(|| false);
    rsx! {
        label {
            class: "d-btn d-btn--primary",
            style: if uploading() { "opacity:.6" } else { "" },
            if uploading() { "Uploading…" } else { "+ Add files" }
            input {
                r#type: "file",
                accept: ".wav",
                style: "display:none",
                disabled: uploading(),
                onchange: move |evt| {
                    spawn(async move {
                        uploading.set(true);
                        for file in evt.files() {
                            let name = file.name();
                            if let Ok(bytes) = file.read_bytes().await {
                                let meta = AddTrackRequest { title: name, artist: String::new(), album: String::new() };
                                let chunks = build_upload_chunks(meta, bytes.to_vec());
                                let _ = client().upload_track(futures_util::stream::iter(chunks)).await;
                            }
                        }
                        uploading.set(false);
                        on_uploaded.call(());
                    });
                },
            }
        }
    }
}

/// Splits an uploaded file's bytes into `UploadTrackChunk`s: metadata on the first message only,
/// `is_final` on the last -- matching the .proto's own doc for the field. tonic-web-wasm-client
/// collects this whole stream into one browser fetch body before sending it (confirmed by reading
/// its source -- there is no incremental client-streaming request body from a browser), so this is
/// really "one upload, several logical messages," not a byte-for-byte network stream; still
/// correct, since the server's own UploadTrack handler (rpc.go) just reads messages off the stream
/// it's given, not caring how they arrived on the wire.
fn build_upload_chunks(meta: AddTrackRequest, data: Vec<u8>) -> Vec<UploadTrackChunk> {
    const CHUNK_SIZE: usize = 256 * 1024;
    let mut chunks: Vec<UploadTrackChunk> = data
        .chunks(CHUNK_SIZE)
        .enumerate()
        .map(|(i, slice)| UploadTrackChunk {
            metadata: if i == 0 { Some(meta.clone()) } else { None },
            data: slice.to_vec(),
            is_final: false,
        })
        .collect();
    match chunks.last_mut() {
        Some(last) => last.is_final = true,
        None => chunks.push(UploadTrackChunk { metadata: Some(meta), data: Vec::new(), is_final: true }),
    }
    chunks
}

#[component]
fn NowPlaying(
    track: Track,
    state: PlayerState,
    is_playing: bool,
    on_play: Callback<Track>,
    on_pause: EventHandler<()>,
    on_seek: EventHandler<i64>,
    on_volume: EventHandler<i32>,
    on_close: EventHandler<()>,
    on_delete: EventHandler<()>,
) -> Element {
    let title = display_title(&track);
    let by = [track.artist.clone(), track.album.clone()].into_iter().filter(|s| !s.is_empty()).collect::<Vec<_>>().join(" · ");
    let position_ms = if state.track_id == track.id { state.position_ms } else { 0 };
    let volume_pct = state.volume_pct;
    let track_for_play = track.clone();

    rsx! {
        Drawer { label: "Now playing".to_string(), on_close: move |_| on_close.call(()),
            div { style: "display:flex; flex-direction:column; gap:16px",
                div {
                    h2 { style: "margin:0; font-family:var(--font-mono); font-size:17px; font-weight:600", "{title}" }
                    if !by.is_empty() {
                        div { style: "color:var(--dim); font-size:13px; margin-top:2px", "{by}" }
                    }
                }

                div {
                    // A real, seekable position slider, not a mock waveform -- see this file's own
                    // doc comment for why there's no peak-data waveform rendering here.
                    input {
                        r#type: "range",
                        min: "0",
                        max: "{track.duration_ms.max(1)}",
                        value: "{position_ms}",
                        style: "width:100%; accent-color:var(--primary)",
                        aria_label: "Seek",
                        oninput: move |ev| {
                            if let Ok(v) = ev.value().parse::<i64>() {
                                on_seek.call(v);
                            }
                        },
                    }
                    div {
                        style: "display:flex; justify-content:space-between; font-family:var(--font-mono); font-size:11px; color:var(--dim); margin-top:2px",
                        span { "{format_duration_ms(position_ms)}" }
                        span { "{format_duration_ms(track.duration_ms)}" }
                    }
                }

                div { style: "display:flex; align-items:center; justify-content:center; gap:12px",
                    if is_playing {
                        button {
                            class: "d-btn d-btn--primary",
                            style: "width:52px; height:52px; border-radius:50%; font-size:16px; padding:0",
                            aria_label: "Pause",
                            onclick: move |_| on_pause.call(()),
                            "❚❚"
                        }
                    } else {
                        button {
                            class: "d-btn d-btn--primary",
                            style: "width:52px; height:52px; border-radius:50%; font-size:16px; padding:0",
                            aria_label: "Play",
                            onclick: move |_| on_play.call(track_for_play.clone()),
                            "▶"
                        }
                    }
                }

                label { class: "d-field",
                    span { class: "d-label", "Volume" }
                    input {
                        r#type: "range",
                        min: "0",
                        max: "100",
                        value: "{volume_pct}",
                        style: "width:100%; accent-color:var(--primary)",
                        oninput: move |ev| {
                            if let Ok(v) = ev.value().parse::<i32>() {
                                on_volume.call(v);
                            }
                        },
                    }
                }

                label { class: "d-field",
                    span { class: "d-label", "Output" }
                    // Fixed, not a dropdown -- see this file's own doc comment for why.
                    span { style: "font-size:13px; color:var(--dim)", "Stream to browser" }
                }

                div {
                    span { class: "d-label", "Details" }
                    dl { class: "d-kv", style: "margin-top:6px",
                        dt { "size" } dd { "{format_bytes(track.size_bytes)}" }
                        dt { "format" } dd { "{track.format}" }
                    }
                }

                Btn { variant: BtnVariant::Danger, onclick: move |_| on_delete.call(()), "Delete" }
            }
        }
    }
}
