//! Record: capture from the browser's own microphone, watch live captions arrive, add markers,
//! stop to finalize. Real meters, real transcript, no mock data.
//!
//! **A real, load-bearing gap found while building this, not a Phase 10 bug**: `StreamAudioIn`
//! (the client-streaming RPC Phase 4 designed for exactly this) cannot actually deliver live audio
//! incrementally from a browser -- `tonic-web-wasm-client`'s own `fetch()`-based transport
//! collects an entire client-streaming request into one call before sending anything (confirmed by
//! reading its source), so a live, open-ended recording sent that way would never reach the server
//! until the connection closed. This view instead calls the new `StreamAudioInChunk` unary RPC
//! once per captured buffer -- see its own `.proto` doc.
//!
//! Real mic capture and real meters both come from the same place: a single `document::eval`
//! running `getUserMedia`/`AudioContext`/`ScriptProcessorNode` inline (so `dioxus.send()` is in
//! scope inside the audio callback -- a separately-loaded script, like `audio-player.js`, has no
//! access to that specific `eval` call's own channel). Each captured buffer's peak level goes back
//! to Rust alongside its PCM in the same message, driving a real `Sparkline` level history -- not
//! a canvas animation standing in for one.

use base64::{engine::general_purpose::STANDARD, Engine};
use dioxus::document::Eval;
use dioxus::prelude::*;
use draft_api::proto::tooling_relay_v1::{
    AddMarkerRequest, AudioChunk, Recording, StartRecordingRequest, StopRecordingRequest,
    TranscriptSegment, WatchTranscriptRequest,
};
use draft_ui::layout::PageHead;
use draft_ui::ui::{Btn, Field, TextInput};
use draft_ui::viz::Sparkline;
use gloo_timers::future::TimeoutFuture;
use serde::Deserialize;

use crate::api::client;

/// How many recent level samples the Sparkline shows -- roughly the last 25s at one ~256ms
/// ScriptProcessorNode callback (4096 samples @ 16kHz) per sample.
const LEVEL_HISTORY_LEN: usize = 100;

/// One message from the capture script -- untagged since each variant has a distinct field shape
/// (`{pcm, db}` vs `{done}` vs `{error}`), which serde can dispatch on without an explicit tag.
#[derive(Deserialize)]
#[serde(untagged)]
enum CaptureMsg {
    Chunk { pcm: String, db: f64 },
    // The bool payload is never read -- the message's own shape (just {done}) is what distinguishes
    // this variant from Chunk/Error for the untagged deserializer; matching on the variant is enough.
    #[allow(dead_code)]
    Done { done: bool },
    Error { error: String },
}

/// getUserMedia -> a 16kHz mono AudioContext -> ScriptProcessorNode, converting each buffer to
/// PCM16LE and sending it (base64) plus its peak dBFS back to Rust via `dioxus.send()`. A silent
/// gain node keeps the graph "active" (some browsers won't fire `onaudioprocess` without some path
/// to `destination`) without actually looping the mic back to the speakers. Runs until Rust sends
/// `{cmd: "stop"}` via `dioxus.recv()`, then tears everything down and reports `{done: true}`.
const CAPTURE_JS: &str = r#"
(async () => {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const ctx = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: 16000 });
    const source = ctx.createMediaStreamSource(stream);
    const processor = ctx.createScriptProcessor(4096, 1, 1);
    const silentGain = ctx.createGain();
    silentGain.gain.value = 0;
    source.connect(processor);
    processor.connect(silentGain);
    silentGain.connect(ctx.destination);

    processor.onaudioprocess = (e) => {
      const input = e.inputBuffer.getChannelData(0);
      const pcm = new Int16Array(input.length);
      let peak = 0;
      for (let i = 0; i < input.length; i++) {
        const s = Math.max(-1, Math.min(1, input[i]));
        pcm[i] = s < 0 ? s * 32768 : s * 32767;
        peak = Math.max(peak, Math.abs(s));
      }
      const bytes = new Uint8Array(pcm.buffer);
      let binary = "";
      for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
      const db = peak > 0 ? 20 * Math.log10(peak) : -60;
      dioxus.send({ pcm: btoa(binary), db: db });
    };

    while (true) {
      const cmd = await dioxus.recv();
      if (cmd && cmd.cmd === "stop") break;
    }

    processor.disconnect();
    source.disconnect();
    silentGain.disconnect();
    stream.getTracks().forEach((t) => t.stop());
    await ctx.close();
    dioxus.send({ done: true });
  } catch (err) {
    dioxus.send({ error: String(err) });
  }
})();
"#;

/// "Recording · Sep 30, 2026, 9:15 PM" -- a reasonable starting point the user can overwrite
/// before hitting Start, not something they're required to type past. `js_sys::Date` rather than
/// `chrono::Local` since nothing else in this crate needs wall-clock time and the JS engine
/// already has the user's real local timezone for free.
fn default_recording_name() -> String {
    format!("Recording · {}", js_sys::Date::new_0().to_locale_string("en-US", &js_sys::Object::new()))
}

#[component]
pub fn Record() -> Element {
    let mut session: Signal<Option<Recording>> = use_signal(|| None);
    let mut recording_name: Signal<String> = use_signal(default_recording_name);
    let mut capture_eval: Signal<Option<Eval>> = use_signal(|| None);
    let mut capture_error: Signal<Option<String>> = use_signal(|| None);
    let mut level_history: Signal<Vec<f64>> = use_signal(Vec::new);
    let mut current_db: Signal<f64> = use_signal(|| -60.0);
    let mut transcript: Signal<Vec<TranscriptSegment>> = use_signal(Vec::new);
    let mut elapsed_ms: Signal<i64> = use_signal(|| 0);
    let mut marker_count = use_signal(|| 0u32);

    let word_count: Memo<usize> = use_memo(move || {
        transcript.read().iter().map(|s| s.text.split_whitespace().count()).sum()
    });

    let on_start = move |_| {
        spawn(async move {
            capture_error.set(None);
            // An empty/whitespace-only name falls back to the same default shown as the
            // placeholder -- never send StartRecording a blank name just because the user cleared
            // the field without typing a replacement.
            let name = recording_name.peek().trim().to_string();
            let name = if name.is_empty() { default_recording_name() } else { name };
            let resp = match client().start_recording(StartRecordingRequest {
                name,
                input_device_id: String::new(),
            }).await {
                Ok(r) => r.into_inner(),
                Err(e) => {
                    capture_error.set(Some(format!("failed to start recording: {e}")));
                    return;
                }
            };
            let recording_id = resp.id.clone();
            session.set(Some(resp));
            level_history.set(Vec::new());
            transcript.set(Vec::new());
            elapsed_ms.set(0);
            marker_count.set(0);

            // Elapsed timer -- runs until this recording is no longer the active one.
            let rid = recording_id.clone();
            spawn(async move {
                loop {
                    TimeoutFuture::new(100).await;
                    if session.peek().as_ref().map(|r| r.id.as_str()) != Some(rid.as_str()) {
                        break;
                    }
                    *elapsed_ms.write() += 100;
                }
            });

            // Live transcript -- WatchTranscript is real server-streaming, which grpc-web (unlike
            // client-streaming) supports incrementally over a browser fetch response body.
            let rid = recording_id.clone();
            spawn(async move {
                let Ok(resp) = client().watch_transcript(WatchTranscriptRequest { recording_id: rid.clone() }).await else {
                    return;
                };
                let mut stream = resp.into_inner();
                while let Ok(Some(seg)) = stream.message().await {
                    if session.peek().as_ref().map(|r| r.id.as_str()) != Some(rid.as_str()) {
                        break;
                    }
                    transcript.write().push(seg);
                }
            });

            // Real mic capture -- see CAPTURE_JS's own doc comment for why this is one big inline
            // eval rather than a separately-loaded script.
            let mut eval = document::eval(CAPTURE_JS);
            capture_eval.set(Some(eval));
            loop {
                let msg: Result<CaptureMsg, _> = eval.recv().await;
                match msg {
                    Ok(CaptureMsg::Chunk { pcm, db }) => {
                        current_db.set(db);
                        let mut hist = level_history.write();
                        hist.push((db + 60.0).max(0.0) / 60.0); // normalize -60..0 dBFS to 0..1
                        if hist.len() > LEVEL_HISTORY_LEN {
                            let excess = hist.len() - LEVEL_HISTORY_LEN;
                            hist.drain(0..excess);
                        }
                        drop(hist);

                        if let Ok(bytes) = STANDARD.decode(&pcm) {
                            let _ = client().stream_audio_in_chunk(AudioChunk {
                                pcm: bytes,
                                sequence: 0,
                                is_final: false,
                            }).await;
                        }
                    }
                    Ok(CaptureMsg::Error { error }) => {
                        capture_error.set(Some(format!("microphone error: {error}")));
                        break;
                    }
                    Ok(CaptureMsg::Done { .. }) | Err(_) => break,
                }
            }
        });
    };

    let on_stop = move |_| {
        let Some(rec) = session.peek().clone() else { return };
        if let Some(eval) = capture_eval.peek().as_ref() {
            let _ = eval.send(serde_json::json!({"cmd": "stop"}));
        }
        spawn(async move {
            let _ = client().stop_recording(StopRecordingRequest { recording_id: rec.id.clone() }).await;
            session.set(None);
            capture_eval.set(None);
            // Fresh default for the next recording -- otherwise this one's (possibly hand-typed)
            // name would silently carry over as the next recording's starting point too.
            recording_name.set(default_recording_name());
        });
    };

    let on_marker = move |_| {
        let Some(rec) = session.peek().clone() else { return };
        spawn(async move {
            if client().add_marker(AddMarkerRequest { recording_id: rec.id.clone(), label: "Marker".to_string() }).await.is_ok() {
                marker_count += 1;
            }
        });
    };

    let is_recording = session().is_some();
    let level_percent = ((current_db() + 60.0).max(0.0) / 60.0 * 100.0).min(100.0);

    rsx! {
        PageHead {
            title: "Record".to_string(),
            eyebrow: "Capture · voice to text".to_string(),
            description: "Records from the browser's own microphone; the transcript streams in below as it's finalized.".to_string(),
            actions: rsx! {
                if is_recording {
                    Btn { onclick: on_marker, "Add marker ({marker_count})" }
                }
            },
        }

        if let Some(err) = capture_error() {
            div { class: "d-panel", style: "padding:12px 16px; margin-bottom:16px; color:var(--err)", "{err}" }
        }

        div { class: "d-panel", style: "padding:20px; margin-bottom:16px; display:flex; flex-direction:column; gap:16px",
            if is_recording {
                div { style: "font-size:15px; font-weight:600",
                    "{session().map(|r| r.name).unwrap_or_default()}"
                }
            } else {
                Field { label: "Name".to_string(), hint: None, error: None,
                    TextInput {
                        value: recording_name(),
                        oninput: move |v| recording_name.set(v),
                        placeholder: default_recording_name(),
                        aria_label: "Recording name".to_string(),
                    }
                }
            }
            div { style: "display:flex; align-items:center; gap:16px",
                if is_recording {
                    button {
                        class: "d-btn d-btn--danger",
                        style: "width:56px; height:56px; border-radius:50%",
                        aria_label: "Stop recording",
                        onclick: on_stop,
                        "■"
                    }
                } else {
                    button {
                        class: "d-btn d-btn--primary",
                        style: "width:56px; height:56px; border-radius:50%",
                        aria_label: "Start recording",
                        onclick: on_start,
                        "●"
                    }
                }
                div {
                    div { style: "font-family:var(--font-mono); font-size:24px; font-weight:600", "{format_elapsed(elapsed_ms())}" }
                    if is_recording {
                        small { style: "color:var(--err); letter-spacing:.08em", "● REC" }
                    }
                }
            }

            if is_recording {
                div {
                    span { class: "d-label", "Input level · MacBook mic · browser" }
                    div { style: "margin-top:6px", Sparkline { values: level_history() } }
                    div { style: "font-family:var(--font-mono); font-size:11px; color:var(--dim); margin-top:4px",
                        "{current_db():.1} dBFS"
                    }
                    div { style: "height:6px; border-radius:3px; background:var(--panel-2); overflow:hidden; margin-top:4px",
                        div { style: "height:100%; width:{level_percent}%; background:var(--primary)" }
                    }
                }
            }
        }

        if is_recording {
            div { class: "d-stats", style: "margin-bottom:16px",
                div { class: "d-panel d-stat", span { class: "d-label", "Words" } div { class: "d-stat-value", "{word_count()}" } }
                div { class: "d-panel d-stat", span { class: "d-label", "Markers" } div { class: "d-stat-value", "{marker_count()}" } }
            }
        }

        div { class: "d-panel",
            div { class: "d-panel-head", span { class: "d-label", "Transcript · live" } }
            div { style: "padding:6px 18px 18px; max-height:360px; overflow:auto",
                if transcript().is_empty() {
                    p { style: "color:var(--dim); font-size:13px", if is_recording { "Listening…" } else { "Start recording to see live captions here." } }
                } else {
                    for seg in transcript() {
                        div { key: "{seg.id}", style: "padding:10px 0; border-bottom:1px solid var(--rule)",
                            time { style: "font-family:var(--font-mono); font-size:11.5px; color:var(--dim)", "{format_ms_clock(seg.start_ms)}" }
                            p { style: "margin:4px 0 0", "{seg.text}" }
                        }
                    }
                }
            }
        }
    }
}

fn format_elapsed(ms: i64) -> String {
    let total_seconds = ms.max(0) / 1000;
    let tenths = (ms.max(0) % 1000) / 100;
    format!("{:02}:{:02}.{}", total_seconds / 60, total_seconds % 60, tenths)
}

fn format_ms_clock(ms: i64) -> String {
    let total_seconds = ms.max(0) / 1000;
    format!("{:02}:{:02}:{:02}", total_seconds / 3600, (total_seconds / 60) % 60, total_seconds % 60)
}
