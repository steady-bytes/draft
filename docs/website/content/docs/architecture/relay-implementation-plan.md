---
weight: 47
title: 'Relay — Audio Plugin Implementation Plan'
description: 'System design, data model, and RPC interface for Relay, a Draft tooling-domain service that plays and records audio anywhere in the cluster, transcribes what it captures, and turns action items into Lineman tasks.'
icon: 'graphic_eq'
draft: false
toc: true
---

{{< alert context="info" text="Phases 1-9 complete, Phase 10 in progress as of 2026-09-30 — the backend (Phases 1-9) is fully built: library + browser-playback, recording + live transcription against a real whisper.cpp sidecar, full-text search, action items promoted to real Lineman tasks, Foundry catalog + Beacon wide events, speaker diarization (pipeline built and verified; the pretrained model itself awaits the user's own HuggingFace token), and real PortAudio device listing behind an opt-in build tag. The Dioxus web client (Phase 10) has its Library and Record pages done and live-verified end-to-end (real upload/playback, real mic capture + live transcription) — Transcripts is not built yet. See the Implementation Plan's per-phase notes." />}}

Relay is the audio service depicted in `mockups/pages/relay-library.html`, `relay-record.html`, and
`relay-transcripts.html`: one service that owns every audio input and output device in the
cluster, so other things ask *it* for audio instead of opening devices themselves — literally
stated in `relay-record.html`'s own mock transcript ("So one service owns every input and every
output, and other services ask it for audio instead of opening devices themselves?" / "Right.").
It has three jobs, matching the mockups' own IA (Listen / Capture):

- **Listen** — a library of stored audio (music, prior recordings) that can be played to any
  output device anywhere in the cluster, with transport controls, volume, and a queue.
- **Capture** — record from any input device, with live dBFS meters and a scrolling waveform,
  streamed to a speech-to-text pipeline in real time (speaker labels, confidence, live captions).
- **Archive** — every recording is full-text searchable by what was said, with hit markers
  plotted on the recording's own waveform, and action items promotable to real Lineman tasks with
  one click.

Relay is in the **tooling** domain (`services/tooling/relay`), alongside Bench, Foundry, and
Lineman — not a core cluster service.

## Relationship to Lineman and Beacon

Relay doesn't duplicate either. **Lineman** owns tasks; Relay only ever calls its existing
`CreateTask` RPC (`api/tooling/lineman/v1/service.proto`) when a user promotes an action item —
Relay has no task/board model of its own, and doesn't watch Lineman for anything. **Beacon**
receives the same `chassis.StartSpan`-scoped wide events every other service produces (request
tracing, business attributes) — it is not where transcripts live or where transcript search runs;
`relay.transcript.segment` CloudEvents on Catalyst are a separate, domain-specific channel Beacon
happens to also be a candidate consumer of, alongside anything else in the cluster that wants to
react to speech in real time.

## System Design

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                                Draft Cluster                                   │
│                                                                                  │
│  ┌───────────┐   ┌──────────┐   ┌───────────┐   ┌────────────┐  ┌────────────┐ │
│  │ Blueprint │   │   Fuse   │   │ Catalyst  │   │   Beacon   │  │  Lineman   │ │
│  │ (registry,│   │ (routing,│   │ (events)  │   │ (telemetry)│  │ (tasks)    │ │
│  │ device    │   │  UI host)│   │           │   │            │  │            │ │
│  │ metadata) │   │          │   │           │   │            │  │            │ │
│  └─────┬─────┘   └────┬─────┘   └─────┬─────┘   └─────┬──────┘  └─────┬──────┘ │
│        │              │               │               │               ▲       │
│        │     ┌────────▼───────────────▼───────────────▼──────┐        │       │
│        └────▶│         Relay (one instance per node with      │        │       │
│               │         audio hardware to expose)              │        │       │
│               │  services/tooling/relay                        │        │       │
│               │  - RelayService RPCs (library, transport,       │        │       │
│               │    recording, transcripts, devices)             │        │       │
│               │  - local device I/O (this node's own mics/     │        │       │
│               │    speakers) + browser-streamed virtual devices │        │       │
│               │  - speech-to-text sidecar (whisper.cpp)         │───┐    │       │
│               │  - Postgres: tracks, recordings, transcripts,   │   │    │       │
│               │    speakers, markers, action items              │   │    │       │
│               │  - local disk: audio bytes (relay-library/,     │   │    │       │
│               │    relay-recordings/), per instance              │   │    │       │
│               │  - Dioxus web client (relay.draft.localhost)  ──┼───┘ CreateTask │
│               └──────────────────────┬──────────────────────────┘   on demand   │
│                                       │ publishes manifest                       │
│                                ┌──────▼──────┐                                   │
│                                │   Foundry    │                                   │
│                                │  (catalog)  │                                   │
│                                └─────────────┘                                   │
└────────────────────────────────────────────────────────────────────────────────┘
```

- **Blueprint** is where every Relay instance registers, exactly like Blueprint's own raft nodes
  or any other service — but unlike a stateless service, *which* instance you're talking to
  matters: each instance advertises the devices physically attached to its own node (see
  [Multi-instance device ownership](#multi-instance-device-ownership-the-hard-part)). Relay's own
  metadata/transcript/track records live in **Postgres** (via `pkg/repositories/postgres/bun`,
  the same as `crud`/`bench`/`foundry`), not Blueprint's KV store — see
  [Decisions](#decisions) for why this departs from Lineman's "everything in Blueprint" approach.
- **Catalyst** carries one `relay.transcript.segment` CloudEvent per finalized transcript segment,
  for any cluster consumer that wants live speech data — matching the `examples.crud.v1.ModelEvent`
  pattern already built for `crud-event`. The live transcript panel on Relay's own Record page does
  **not** consume this — see [Decisions](#decisions).
- **Fuse** routes `relay.draft.localhost` to Relay's own mux (RPC + web client on one route), the
  same [subdomain convention](/docs/architecture/service-ui-subdomains) every other service's UI
  uses. Because multiple Relay instances register, Fuse's existing same-name-route load-balancing
  (the same mechanism serving Blueprint's own UI across 5 raft nodes) fans requests across whichever
  instances are up — fine for the web client and library/search RPCs, which don't care which
  instance answers; playback/recording calls that must reach a *specific* instance (the one
  physically holding the requested device) address that instance directly via the address Blueprint
  returns for it, bypassing Fuse's load-balancing for those calls only.
- **Lineman** gets one write: `CreateTask` per promoted action item, called directly from Relay's
  Transcripts page's "Create N Lineman tasks" button. No polling, no subscription, no coupling
  beyond that one RPC call.
- **Foundry** gets a published plugin manifest at startup (`chassis.Effect`, retracted on shutdown)
  — for discovery/distribution only, the exact pattern
  [Lineman already established](/docs/architecture/lineman-implementation-plan#non-goals). Relay
  does **not** implement `StepExecutor` — it's not a Bench workflow step, it's a standalone
  application with its own UI, the same shape as Lineman itself.
- **The web client** is Dioxus, following Lineman/Blueprint's pattern (own `web-client/` crate,
  `NavigationConfig`-free static rail matching the mockups' fixed IA, `draft-ui` components
  throughout). Not fully specced page-by-page here — the three mockups already are the spec for
  Library, Record, and Transcripts; Queue/Playlists/Inputs/Outputs appear in the mockups' rail nav
  but have no mocked-up page yet and are out of scope for the phases below (see
  [Non-Goals](#non-goals)).

## Multi-instance device ownership (the hard part)

Every other multi-instance pattern in this repo is either stateless (any instance of `crud`
answers any request identically) or raft-consensus (Blueprint's 5 nodes agree on one shared state).
Relay is neither: a microphone or speaker is a **physical resource attached to one specific
machine**, so "Relay" as a logical service is really N independent instances, each fronting the
hardware on its own node, discoverable as one thing but addressed individually for anything that
touches a real device.

- Each instance registers with Blueprint under the same service name (`relay`), the same way
  Blueprint's own raft nodes all register as `blueprint` — Blueprint's `Query` already returns
  every instance of a name with its own `Pid`/`IpAddress`, no new registry mechanism needed.
- On startup, an instance enumerates its own local audio hardware and exposes it via a
  `ListDevices` RPC (its *own* devices only — there is no cluster-wide device registry to keep in
  sync, which sidesteps an entire class of consistency problems Blueprint's raft nodes have to
  solve and Relay doesn't need to).
- The web client (or any RPC caller wanting the full device picker) calls `Query` against
  Blueprint for every `relay` instance, then calls `ListDevices` on each and merges the results —
  exactly the aggregation `services/core/blueprint/web-client/src/views/service_registry.rs`
  already does for the process list, just fanned out over N `ListDevices` calls instead of read
  from one `Watch` stream.
- A call that must touch a specific device (`Play`, `StartRecording`) is sent directly to the
  instance that owns it, addressed by the `IpAddress` Blueprint's `Query`/`Watch` already returns
  for that `Pid` — never broadcast, never routed through Fuse's load-balanced same-name routing
  (which would non-deterministically land on the *wrong* instance for a device-specific call).
- **Local, native hardware I/O needs a real OS audio library** — Go has none in its standard
  library. The concrete recommendation is `github.com/gordonklaus/portaudio` (cgo bindings to
  PortAudio, cross-platform, the most mature option for this in Go) for any instance that runs on
  a machine with real, physically-attached mics/speakers to expose (matching the mockups' "USB mic
  · node-2", "Studio monitors · node-2", "Office speakers · node-4").
- **The browser is also a valid "device"** — the mockups list "MacBook mic · browser" as an input
  and "Stream to browser" as an output alongside real hardware. This needs no PortAudio/cgo at
  all: the Dioxus web client captures via the browser's own Web Audio/MediaRecorder APIs and
  streams raw PCM to whichever Relay instance is doing the actual recording over a Connect
  client-streaming RPC (`StreamAudioIn`), and receives playback the same way in reverse
  (`StreamAudioOut`, server-streaming). This is why [Phase ordering](#implementation-plan) below
  builds the whole product against browser-streamed audio first and treats native PortAudio
  device support as a later, additive phase — it's real, load-bearing scope the mockups depict,
  but it's also the one part of this plan an MVP can defer without faking or cutting anything the
  mockups show for a single-node, browser-mediated setup.

## Speech-to-text pipeline

Nothing in this repo talks to real audio hardware or does speech recognition today — this is
genuinely new capability, not an extension of an existing pattern.

**Recommendation: `whisper.cpp` as a sidecar process, not a cgo-linked library and not a cloud
API.** The mockups' own model picker ("whisper · small.en", "whisper · medium") names
`whisper.cpp`/OpenAI Whisper model sizes directly, which is a strong signal this was designed
around local, self-hosted Whisper rather than a cloud transcription API — consistent with every
other capability in this cluster running locally with no external network dependency. Running it
as its own OS process rather than a cgo-linked library keeps Relay's own build simple (no cgo
cross-compilation concerns for the main service binary) and matches this repo's overall philosophy
of small, separate, individually restartable processes (Catalyst's Produce/Consume, every Foundry
plugin, `catalyst-produce`, etc.) over one process doing everything. Audio chunks stream to the
sidecar as they're captured; partial segments come back for the live caption feed, finalized
segments (with speaker label and confidence) get persisted and produced to Catalyst.

**Sidecar packaging: native binary for local dev, container for production — both speaking the
same HTTP contract, chosen by a config value, not two different integration designs.** Both
`whisper.cpp` and the diarization sidecar below sit behind a plain `http://localhost:<port>`
address Relay's Go backend calls; how that address gets populated is the only thing that differs:

- **Local dev (macOS): a native binary**, built from source (a new prerequisite documented
  alongside Go/Docker/buf/dctl in CLAUDE.md) and started/watched by `run-local-watch.sh` like
  every other process here. This matters specifically for `whisper.cpp`: it has a Metal backend
  for Apple Silicon GPU acceleration, and Docker Desktop on macOS cannot pass Metal through to a
  container (containers run inside a Linux VM there) — so a native binary is meaningfully faster
  for live transcription during dev on this machine, which matters given the mockup's own
  latency stat ("Latency 420ms") treats this as something worth watching.
- **Production (Kubernetes, per CLAUDE.md's own prerequisites): a container**, run as an actual
  Kubernetes sidecar container in Relay's own Pod spec, reachable over `localhost` inside the
  Pod's shared network namespace — no service discovery needed for either sidecar at all. On
  Linux, GPU passthrough to a container (`nvidia-docker`/the NVIDIA device plugin) works fine, so
  this loses nothing production-side.
- **Local dev, container variant (optional, for testing the prod path without a full K8s setup):**
  the exact `docker run -d -p <port>:<port> ...` pattern `run-local.sh` already uses for
  Postgres/ClickHouse — useful for parity-testing before a real deploy, not the default local
  loop.
- `whisper.cpp` ships an official `server` example with an OpenAI-compatible HTTP API
  (`/inference`) — existing community Docker images already package it, so its container path is
  close to turnkey.
- **Not vendored.** Both sidecars are treated as external prerequisites, the same way `buf`,
  `watchexec`, and `dctl` already are — a documented `git clone` + `make` step (pinned to a
  specific tagged release, not "whatever's latest") producing a binary that ends up on `PATH`,
  not source checked into this repo. Model weights (100MB–3GB depending on size) are downloaded
  once via `whisper.cpp`'s own `models/download-ggml-model.sh` into a gitignored local directory —
  the same treatment `.local-stack/`'s own untracked runtime state already gets — never committed
  to git.

**Speaker diarization** (telling "Andrew" from "Speaker 2") is a second, genuinely hard capability
`whisper.cpp` doesn't do on its own, and it changes what "done" means for two different phases —
worth stating precisely rather than leaving implicit:

- **Recommended approach: `pyannote.audio` as its own sidecar**, following the exact same
  native-dev/container-prod split above. Unlike `whisper.cpp`, `pyannote.audio` has no
  off-the-shelf server binary, so this needs a small purpose-built HTTP wrapper around it —
  `POST /diarize`, taking the finished recording's audio (WAV bytes, or a shared-volume path since
  the sidecar is colocated with Relay in the same Pod) and returning
  `[{start_ms, end_ms, speaker_label}]` — Relay's first actual Python component.
- **Diarization is batch-only, by design, not as a stopgap.** `pyannote`'s pretrained pipeline
  processes a complete audio file; there is no turnkey *streaming* diarization library today.
  So: **Phase 4's live Record page never gets real per-speaker labels** — every live segment shows
  as a single unlabeled speaker, permanently, not just until Phase 8 lands. **Phase 8's
  diarization pass runs once, after `StopRecording` finalizes the file**, and retroactively
  relabels the already-stored segments — so it's specifically the *archive* (Transcripts page)
  that gets correct per-speaker attribution, never the live view. This is a real, intentional
  scope boundary the mockups' own live-labeled Record page doesn't depict, called out explicitly
  so Phase 4's and Phase 8's deliverables aren't read as claiming more than they build.
- **External dependency, worth knowing about upfront:** `pyannote`'s pretrained pipeline is gated
  on HuggingFace — the first-ever model download needs a HuggingFace account and access token
  (a one-time setup step, like accepting the model's terms). Inference itself runs fully locally
  afterward, but this is a small crack in an otherwise fully-local, no-external-account story,
  worth knowing about before Phase 8 starts rather than discovering it mid-implementation.

**Action item detection** (the Transcripts page's "Action items found · 2" list) needs either an
LLM call over the finished transcript, or a much simpler heuristic/manual-only approach for a
first pass. See [Decisions](#decisions) for the recommended phasing.

## Data Model

### Proto

New package `tooling.relay.v1` (`api/tooling/relay/v1/models.proto`), following the
`service.proto`/`models.proto` split every tooling-domain API in this repo uses:

```protobuf
message Track {
    string id           = 1;
    string title        = 2;
    string artist       = 3;
    string album        = 4;
    string format       = 5;  // "FLAC 24/96", "MP3 320", "WAV 48k/24-bit", ...
    int64  duration_ms   = 6;
    int64  size_bytes    = 7;
    string storage_node  = 8;  // the Relay instance (Pid) whose local disk holds the bytes
    string storage_path  = 9;  // path under that instance's relay-library/ or relay-recordings/
    google.protobuf.Timestamp added_at = 10;
    // Set only for a track that originated from a Relay recording, not an added library file.
    string recording_id  = 11;
}

message Recording {
    string    id           = 1;
    string    name         = 2;   // "Design review · Relay UI"
    string    track_id     = 3;   // the playable Track this recording became once finalized
    RecordingStatus status  = 4;
    int64     duration_ms   = 5;
    int32     word_count    = 6;
    repeated Speaker speakers = 7;
    google.protobuf.Timestamp started_at   = 8;
    google.protobuf.Timestamp finished_at  = 9; // unset while still recording
}

enum RecordingStatus {
    RECORDING_STATUS_UNSPECIFIED = 0;
    RECORDING_STATUS_LIVE        = 1;
    RECORDING_STATUS_COMPLETE    = 2;
    RECORDING_STATUS_LOW_CONFIDENCE = 3; // finished, but STT confidence was low throughout
}

message Speaker {
    string id    = 1;
    string label = 2; // "Andrew", "Speaker 2" -- user-renameable
}

message TranscriptSegment {
    string id           = 1;
    string recording_id = 2;
    int64  start_ms      = 3;
    int64  end_ms        = 4;
    string speaker_id    = 5;
    string text          = 6;
    float  confidence    = 7;
    bool   is_partial     = 8; // live/streaming segment, not yet finalized
}

message Marker {
    string id           = 1;
    string recording_id = 2;
    int64  at_ms         = 3;
    string label         = 4; // user-added via "Add marker" during Record
}

message ActionItem {
    string id             = 1;
    string recording_id   = 2;
    string text           = 3;
    string source_segment_id = 4;
    int64  at_ms           = 5;
    string speaker_label   = 6;
    string lineman_task_id = 7; // set once promoted via CreateLinemanTasks
}

// Device is never stored -- it's always a live read of one Relay instance's own local hardware
// (see ListDevices), included here only to name the shape every instance returns.
message Device {
    string id   = 1;
    string name = 2; // "USB mic · node-2 · mono", "Studio monitors · node-2 · 48 kHz"
    DeviceKind kind = 3;
    int32 channels   = 4;
    int32 sample_rate_hz = 5;
}

enum DeviceKind {
    DEVICE_KIND_UNSPECIFIED = 0;
    DEVICE_KIND_INPUT       = 1;
    DEVICE_KIND_OUTPUT      = 2;
}
```

### Postgres, not Blueprint KV

`Track`/`Recording`/`Speaker`/`TranscriptSegment`/`Marker`/`ActionItem` are stored in Postgres via
`bun` — see [Decisions](#decisions) for why this departs from Lineman's Blueprint-KV-only
approach. `TranscriptSegment.text` needs a full-text index (`tsvector`/`GIN`, matching the
Transcripts page's own `SEARCH "leader election"` query bar) — not something Blueprint's KV store
does at all.

## RPC Interface

`api/tooling/relay/v1/service.proto`:

```protobuf
service RelayService {
    // Devices -- always answered from THIS instance's own local hardware, never aggregated
    // server-side (the caller aggregates across instances; see Multi-instance device ownership).
    rpc ListDevices(ListDevicesRequest) returns (ListDevicesResponse) {}

    // Library
    rpc ListTracks(ListTracksRequest) returns (ListTracksResponse) {}
    rpc AddTrack(AddTrackRequest) returns (Track) {} // metadata; bytes arrive via UploadTrack
    rpc UploadTrack(stream UploadTrackChunk) returns (Track) {}
    rpc RescanLibrary(RescanLibraryRequest) returns (RescanLibraryResponse) {}

    // Playback -- Play/Pause/Seek/SetVolume target a specific output device, so they're always
    // called on the instance that owns it (see Multi-instance device ownership).
    rpc Play(PlayRequest) returns (PlayerState) {}
    rpc Pause(PauseRequest) returns (PlayerState) {}
    rpc Seek(SeekRequest) returns (PlayerState) {}
    rpc SetVolume(SetVolumeRequest) returns (PlayerState) {}
    rpc GetPlayerState(GetPlayerStateRequest) returns (PlayerState) {}
    rpc StreamAudioOut(StreamAudioOutRequest) returns (stream AudioChunk) {} // "Stream to browser" output

    // Queue
    rpc Enqueue(EnqueueRequest) returns (QueueState) {}
    rpc ClearQueue(ClearQueueRequest) returns (QueueState) {}

    // Recording -- always called on the instance owning the input device, same reasoning as
    // Playback. StreamAudioIn is how a browser-captured mic gets its bytes to that instance.
    rpc StartRecording(StartRecordingRequest) returns (Recording) {}
    rpc StreamAudioIn(stream AudioChunk) returns (StreamAudioInResponse) {}
    rpc AddMarker(AddMarkerRequest) returns (Marker) {}
    rpc StopRecording(StopRecordingRequest) returns (Recording) {}
    rpc DiscardRecording(DiscardRecordingRequest) returns (DiscardRecordingResponse) {}

    // Live transcript -- the Record page's own streaming captions. A dedicated RPC, not a
    // Catalyst Consume subscription -- same reasoning as Lineman's Watch; see Decisions.
    rpc WatchTranscript(WatchTranscriptRequest) returns (stream TranscriptSegment) {}

    // Archive
    rpc ListRecordings(ListRecordingsRequest) returns (ListRecordingsResponse) {}
    rpc GetRecording(GetRecordingRequest) returns (Recording) {}
    rpc GetTranscript(GetTranscriptRequest) returns (GetTranscriptResponse) {}
    rpc SearchTranscripts(SearchTranscriptsRequest) returns (SearchTranscriptsResponse) {}
    rpc RenameSpeaker(RenameSpeakerRequest) returns (Speaker) {}
    rpc DeleteRecording(DeleteRecordingRequest) returns (DeleteRecordingResponse) {}

    // Action items
    rpc ListActionItems(ListActionItemsRequest) returns (ListActionItemsResponse) {}
    rpc CreateLinemanTasks(CreateLinemanTasksRequest) returns (CreateLinemanTasksResponse) {}
}

message AudioChunk {
    bytes  pcm             = 1; // raw PCM16LE, sample_rate_hz/channels fixed per stream at open
    int64  sequence         = 2;
    bool   is_final          = 3;
}

message PlayerState {
    string track_id   = 1;
    int64  position_ms = 2;
    bool   playing      = 3;
    int32  volume_pct   = 4;
    string output_device_id = 5;
}

message QueueState {
    repeated Track upcoming = 1;
}
```

No Fuse-specific routing work beyond the one `WithRoute` call in [System Design](#system-design) —
`RelayService` gets its own prefix like every other Connect-RPC service; the multi-instance
addressing described above happens at the *client* layer (the web client, or a future caller),
not inside Fuse.

## Telemetry

Every mutating RPC opens its own `chassis.StartSpan` child span, business attributes set before
`span.End`, matching `crud`/`crud-event`/Lineman exactly:

| RPC | Business attributes |
|---|---|
| `StartRecording` / `StopRecording` | `recording_id`, `input_device_id` |
| `Play` | `track_id`, `output_device_id` |
| `SearchTranscripts` | `query`, `result_count` |
| `CreateLinemanTasks` | `recording_id`, `task_count`, `lineman_task_ids` |

Each finalized `TranscriptSegment` also produces a `relay.transcript.segment` CloudEvent on
Catalyst — `chassis.StartSpan` covers the RPC/pipeline step that finalized it, not the event
delivery itself, the same separation `crud-event`'s `ModelEvent` publish already establishes
(the WideEvent is this instance's own trace; the CloudEvent is a fact for the rest of the
cluster, independent of whether anyone is looking at this instance's trace at all).

## Decisions

**Postgres, not Blueprint's KV store, for tracks/recordings/transcripts.** Lineman's plan
explicitly chose Blueprint-only storage and calls Postgres a non-goal "revisit only if KV proves
inadequate." Relay hits that inadequacy on day one: transcript full-text search
(`SearchTranscripts`, the Transcripts page's own headline feature) needs a real `tsvector`/`GIN`
index, and Blueprint's raft-replicated KV store has no query capability beyond key lookup. Audio
bytes themselves go on local disk, not Postgres or Blueprint either — see next decision.

**Local disk, not a new object-store service, for audio bytes.** Nothing in this repo provides
S3-compatible or other object storage today (`services/examples/file_host` is a static-site host,
not a blob store) — inventing one is out of scope for Relay specifically. Each Relay instance
keeps its own recordings/library files under a configured local directory
(`relay-library/`, `relay-recordings/`, matching the mockups' own bucket-name copy, kept as
directory names instead of literal object-store buckets). A `Track`'s `storage_node` +
`storage_path` say which instance's disk holds it; playing it from a *different* instance means
that instance requests the bytes from the owner over `StreamAudioOut`'s server-streaming shape
rather than assuming shared/networked storage.

**`WatchTranscript` is a dedicated streaming RPC, not a Catalyst `Consume` subscription**, for the
same reason [Lineman's `Watch`](/docs/architecture/lineman-implementation-plan#decisions) is: a
live UI needs reliable, single-recipient delivery. Unlike when Lineman's plan was written,
Catalyst's fan-out bug is now fixed (`services/core/catalyst/broker`, see
[Known issues](/docs/architecture/core-services#known-issues)) — Relay's `relay.transcript.segment`
CloudEvent on Catalyst would now correctly fan out to every subscriber, so this is a genuine
choice rather than a bug-driven workaround: `WatchTranscript` stays a direct RPC because it's
simpler and has zero dependency on Catalyst being up, not because Consume can't be trusted anymore.

**Foundry manifest for discovery only, no `StepExecutor`.** Same reasoning and precedent as
[Lineman's own decision](/docs/architecture/lineman-implementation-plan#non-goals) — Relay is a
standalone application with its own UI, not a Bench workflow step.

**Action item detection starts manual, not LLM-based.** The mockups show "Add marker" during
recording and a finished "Action items found · N" list on Transcripts. Automatically detecting
action items from arbitrary speech is a real NLP problem (most naturally solved with an LLM call
over the finished transcript) that deserves its own scoped follow-up rather than being smuggled
into this plan's first pass. The phased plan below ships markers → manually-promoted action items
first (a marker or a manually-selected transcript excerpt becomes an `ActionItem` a user
explicitly creates), with LLM-based auto-detection as an explicit later phase, not assumed here.

**Native per-node hardware capture (PortAudio) is a later phase, not part of the MVP.** See
[Multi-instance device ownership](#multi-instance-device-ownership-the-hard-part) — the whole
product (Library, Record, Transcripts, Lineman integration) can be built and proven against
browser-streamed audio alone first, with zero cgo/native-library risk, before taking on real
per-node hardware I/O.

**Sidecar packaging: native binary for local dev, Kubernetes sidecar container for production**,
for both `whisper.cpp` and the `pyannote.audio` diarization wrapper. See
[Speech-to-text pipeline](#speech-to-text-pipeline) for the full reasoning (Metal GPU acceleration
on macOS isn't available to Docker Desktop containers there, but GPU passthrough to a container
works fine on Linux/Kubernetes) — confirmed 2026-09-30.

**Speaker diarization is batch-only: the archive gets it, the live Record page never does.**
`pyannote.audio` processes a complete audio file; there's no turnkey streaming diarization library
today. Phase 4's live transcript is permanently single-unlabeled-speaker; Phase 8 runs one
diarization pass per finalized `Recording` and relabels its stored segments retroactively. See
[Speech-to-text pipeline](#speech-to-text-pipeline) — confirmed 2026-09-30, including the
HuggingFace-gated-model caveat that comes with it.

**`whisper.cpp` and `pyannote.audio` are external prerequisites, not vendored.** Same treatment as
`buf`/`watchexec`/`dctl` — a documented, version-pinned build step producing a `PATH` binary, not
source checked into this repo. Model weights are downloaded once into a gitignored local
directory, never committed — confirmed 2026-09-30.

**One shared Postgres database across every Relay instance**, not per-instance storage — tracks,
recordings, and transcripts are cluster-wide concepts regardless of which instance's local disk
holds the underlying audio bytes, matching how `crud`/`crud-event` already share one database
rather than one each — confirmed 2026-09-30.

**One shared `PlayerState` per output device, not per listener.** A physical speaker has exactly
one real playback position and volume regardless of how many browser tabs are looking at it;
`Play`/`Pause`/`Seek`/`SetVolume`/`GetPlayerState` all read and write that single shared state —
confirmed 2026-09-30.

## Non-Goals

- **No object-store service.** See Decisions — local disk per instance instead.
- **No cross-node shared/networked filesystem assumption.** A track's bytes live on exactly one
  instance's local disk; anything needing them from elsewhere streams them over RPC.
- **No live/real-time speaker diarization, ever, not just for a first pass.** `pyannote.audio`
  (the chosen approach — see Decisions) is batch-only; the live Record page permanently shows one
  unlabeled speaker regardless of how many phases ship. Only the finished, archived transcript
  gets real per-speaker labels.
- **No LLM-based action item detection in the first pass.** See Decisions.
- **No native PortAudio/hardware device support in the first pass.** See Decisions; the mockups'
  "USB mic · node-2"/"Studio monitors · node-2" scenarios are real, designed-for scope, just a
  later phase.
- **No auth on any RPC**, matching the system-wide status quo every other service in this repo
  also has today (Lineman, Bench, crud-event, ...) — a `Discard`/`DeleteRecording` call reachable
  by anyone who can reach Relay's UI is a real gap worth a conscious answer before any real
  deployment, not an oversight specific to Relay.
- **No Queue/Playlists/Inputs/Outputs pages.** Referenced in the mockups' own rail nav but never
  mocked up — building them means designing them first, which is its own follow-up, not implied
  by this plan.

## Implementation Plan

- [x] **1. Scaffolding.** `services/tooling/relay`: `go.mod`, `main.go`, `config.yaml`, following
  the `chassis.New(logger).Register(...).WithRepository(db).WithRPCHandler(...).WithRoute(...).Start()`
  shape `crud`/`bench`/Lineman all use. `WithRoute` for `relay.draft.localhost`. Postgres via
  `bun.New("")`, matching `crud`'s own `CreateSchema` pattern. Deliverable: a service that starts,
  registers with Blueprint, connects to Postgres, and does nothing else yet. _(completed
  2026-09-30 — port 9308, own `relay`/`relay` Postgres role+database created (both scripts'
  provisioning blocks and the live already-running container), following bench/foundry's
  per-service-database convention exactly. No `WithRoute`/`WithRPCHandler`/`CreateSchema` yet — none
  are meaningful before Phase 2's proto exists (mirrors Lineman's own Phase 1, which added those
  incrementally per later phase, not all at once). Live-verified: registered and healthy in
  Blueprint's Service Registry (`Query` confirms `PROCESS_HEALTHY`), and a real idle connection
  from the `relay` Postgres role visible in `pg_stat_activity`, confirming `WithRepository(db)`
  actually opened a live connection, not just a valid config.)_

- [x] **2. Proto.** `api/tooling/relay/v1/{models,service}.proto` per [Data Model](#data-model) and
  [RPC Interface](#rpc-interface). Regenerate (`dctl api build`). Deliverable: generated Go/Rust/TS
  types compile; no behavior change yet. _(completed 2026-09-30 — the plan's own RPC Interface
  sketch left most request/response message shapes implicit (only the interesting/novel ones were
  spelled out); filled in the rest as standard List/Get/Create-shaped messages during this phase,
  including one addition the plan didn't call out: `SearchHit` (a matched `Recording` paired with
  just its hit `TranscriptSegment`s), needed to avoid a second round trip for
  `relay-transcripts.html`'s per-row hit list. Also proactively fixed `api/build.rs`'s hardcoded
  `proto_dirs` list (the exact gotcha Lineman's own Phase 2 hit) by adding `tooling/relay/v1/`
  before running codegen, rather than discovering it after. Go build/vet clean; Rust build clean
  (generated output lands in the checked-in `api/src/proto/tooling.relay.v1.rs` and
  `api/src/hook/tooling.relay.v1.dx.rs`, not Cargo's ephemeral build cache — worth remembering,
  cost real time to re-find this session). Web/TS codegen failed the same pre-existing way it does
  for every other tooling-domain proto (confirmed: Lineman's own `workflow.v1`/`plugin_catalog.v1`
  have no `_pb.ts` either) — not something this phase broke.)_

- [x] **3. Library: metadata + playback against browser-streamed/uploaded audio only.** `AddTrack`/
  `UploadTrack`/`ListTracks`, `Play`/`Pause`/`Seek`/`SetVolume`/`GetPlayerState`, `StreamAudioOut`
  serving bytes to the browser (the "Stream to browser" output device — no PortAudio yet).
  Deliverable: upload a file through the web client, see it in the library list, play it back
  through the browser's own audio output with working transport controls, matching
  `relay-library.html`'s UI against real data. _(completed 2026-09-30 — backend only; the Dioxus
  web client itself is Phase 10, verified here instead via a direct streaming RPC client. Two
  scope trims beyond what's written above, both disclosed rather than silent: (1) **WAV-only** --
  `UploadTrack`/playback accept/serve raw PCM via a hand-rolled RIFF/WAVE header parser
  (`service/wav.go`); MP3/FLAC/AAC would need a real decode library and are an explicit follow-up,
  not attempted here. (2) `Track` (which carries a `google.protobuf.Timestamp`) is stored via a
  dedicated `trackRow` bun struct + conversion functions, not used directly as a bun model --
  following `services/tooling/bench/model.go`'s own precedent/reasoning for exactly this case, not
  `crud`'s (whose `Name` has no timestamp field). `UploadTrack` streams straight to a temp file
  under `relay.storage_dir` as chunks arrive (never buffers a whole upload in memory), then moves
  it into place once the WAV header parses successfully. Playback (`StreamAudioOut`) is a
  real-time-paced send loop guarded by a generation counter on `Player` (`service/player.go`) --
  the same coalesce-to-latest-state shape as Blueprint's own `Broadcaster` fix, applied here so a
  concurrent Pause/Seek/Play correctly interrupts an in-flight stream rather than racing it.
  Live-verified end-to-end against a real synthesized WAV through a scratch Connect client:
  `UploadTrack` (WAV header parsed correctly: sample rate/bit depth/duration/size all matched the
  source file) → `ListTracks` → `Play` → `StreamAudioOut` (multiple real `AudioChunk`s received,
  `GetPlayerState.position_ms` advancing in step with real-time pacing) → `Seek` → `Pause`, all
  passing. Also fixed a live bug this phase's own work exposed: `scripts/run-local-watch.sh`'s
  Relay entry used `-w .` (whole-directory watch), which put `UploadTrack`'s own writes into
  `./.relay-storage` inside the watched tree -- every upload restarted the service mid-stream
  (confirmed live: a `StreamAudioOut` client got a hard "connection refused" mid-playback).
  Switched to explicit `-w main.go -w go.mod -w go.sum -w config.yaml -w service`, matching this
  script's own already-documented convention for Blueprint's `tmp/` (badger) and Lineman's
  per-file list. Worth remembering for Phase 4 (Recording), which will write audio files to local
  disk the same way.)_

- [x] **4. Recording + live transcription against browser-captured audio only.** `StartRecording`/
  `StreamAudioIn` (fed by the browser's MediaRecorder/Web Audio capture, the "MacBook mic ·
  browser" input device), the whisper.cpp sidecar integration described in
  [Speech-to-text pipeline](#speech-to-text-pipeline), `WatchTranscript` for live captions,
  `AddMarker`, `StopRecording` finalizing a `Recording` + its `Track`. Diarization is batch-only
  (Phase 8) by design (see [Speech-to-text pipeline](#speech-to-text-pipeline)) — this phase's live
  captions always show one unlabeled speaker, permanently, not as a temporary placeholder.
  Deliverable: record from a real browser microphone, see live captions appear as you speak,
  matching `relay-record.html`'s meters/waveform/transcript panel
  against a real, running capture. _(completed 2026-09-30 — backend only, same disclosed split as
  Phase 3 (the Dioxus web client is Phase 10; verified here via a direct streaming RPC client).
  `whisper.cpp` was actually built and run as a real local sidecar for this phase, not stubbed:
  cloned at tag `v1.7.4` into `.local-stack/whisper.cpp` (gitignored, external prerequisite, not
  vendored — see [Decisions](#decisions)), built with `cmake -DGGML_METAL=ON
  -DWHISPER_BUILD_SERVER=ON`, running its own `server` example (`whisper-server`) against the
  `tiny.en` model on port 9309. `scripts/run-local.sh`/`run-local-watch.sh` now start/watch it
  alongside everything else, with a preflight check that fails fast with the exact one-time setup
  command if it isn't built yet, matching this repo's treatment of `buf`/`watchexec`/`dctl`.
  Live-verified end-to-end against real synthesized speech (macOS `say` piped through `afconvert`
  to 16kHz mono 16-bit, whisper.cpp's own required format): `StartRecording` → `StreamAudioIn`
  (paced small chunks, like a real capture) → a real live segment arrived over `WatchTranscript`
  mid-stream, correct text and a plausible confidence (0.88, derived from whisper.cpp's
  `avg_logprob`) → `AddMarker` (its `at_ms` is the session's own live position -- the request
  carries no timestamp of its own, deliberately, since a marker only makes sense while actually
  recording) → `StopRecording` (status `COMPLETE`, correct `duration_ms`, `word_count` matching
  the real transcript) → the resulting `Track` confirmed in `ListTracks` with `recording_id` set
  → a second `StartRecording` succeeded immediately after (confirms the single-session slot frees
  correctly). One real, disclosed limitation observed live, not just theoretical: fixed
  non-overlapping ~3s transcription windows (not a sliding/growing window, so whisper.cpp's own
  per-call cost never grows with recording length) mean a sentence split across a window boundary
  can come out missing at the seam — confirmed directly against whisper.cpp itself (an isolated
  ~0.75s tail fragment containing only trailing silence/a cut-off phrase transcribed to empty).
  Also added `RecordingSession.Flush` (session.go) after first missing it: without a final flush
  of whatever's accumulated below one full window, `StopRecording` would silently drop up to ~3s
  of trailing audio from the transcript (still present in the finalized `Track`, just untranscribed)
  every time. One deliverable pulled forward from its originally-planned phase: the
  `relay.transcript.segment` CloudEvent publish Phase 7's own checklist item below describes was
  built now (`service/events.go`), since it was natural to wire up alongside segment finalization
  — Phase 7's own remaining scope there is just the Beacon/Foundry side, not the publish itself.)_

- [x] **5. Archive + search.** `ListRecordings`/`GetRecording`/`GetTranscript`, `SearchTranscripts`
  backed by Postgres full-text search, waveform hit-marker data. Deliverable: search across
  multiple real recorded transcripts, get correct hit counts/excerpts, matching
  `relay-transcripts.html`'s list + drawer against real data. _(completed 2026-09-30 — backend
  only, same disclosed split as Phases 3-4 (web client is Phase 10). "Waveform hit-marker data"
  needed no new backend concept: `TranscriptSegment.start_ms`/`end_ms` (already present) is exactly
  what a future waveform view would plot hit markers from — Phase 5's own job was just making sure
  `SearchTranscripts` returns those correctly, not inventing a new field or table. Full-text search
  is a generated `tsvector` column + GIN index on `transcript_segments.text`
  (`CreateRecordingSchema`, `service/recording.go`) — deliberately not a Go struct field, since
  it's computed and queried entirely inside Postgres; this is precisely the capability the plan's
  own "Postgres, not Blueprint KV" decision was written to unlock. `SearchTranscripts` groups
  matching segments by recording (one query for segments, one batched `WHERE id IN (...)` for
  their recordings) into `SearchHit`s, skipping any recording that no longer exists rather than
  failing the whole call. `RenameSpeaker`/`DeleteRecording` are in the same "Archive" RPC group in
  the `.proto` but weren't named in this phase's own deliverable text — left `CodeUnimplemented`
  for now, same precise-scope-matching as Phase 4 leaving `DiscardRecording` alone. Live-verified
  against the real recordings Phase 4 produced: `ListRecordings`/`GetRecording`/`GetTranscript`
  round-trip correctly; `GetRecording` on an unknown id returns `CodeNotFound`, not a bare error;
  `SearchTranscripts` finds real hits for a real phrase, correctly returns zero hits (not an error)
  for a query matching nothing, and — confirming this is genuine Postgres full-text search, not a
  substring match standing in for it — a plural query ("recordings") matches segment text
  containing the singular ("recording") via `to_tsvector`/`plainto_tsquery`'s English stemming.)_

- [x] **6. Action items → Lineman.** `AddMarker`-sourced and manually-selected-excerpt
  `ActionItem`s, `ListActionItems`, `CreateLinemanTasks` calling Lineman's real `CreateTask` RPC.
  Deliverable: select transcript excerpts as action items, click "Create N Lineman tasks," see
  real tasks appear on a real Lineman board. _(completed 2026-09-30 — backend only, same disclosed
  split as Phases 3-5 (web client is Phase 10). One addition beyond the plan's own original RPC
  Interface sketch, found while implementing: nothing in the original proto could ever CREATE an
  `ActionItem` — `ListActionItems`/`CreateLinemanTasks` both assumed one already existed. Added
  `rpc CreateActionItem(CreateActionItemRequest) returns (ActionItem)` (recording_id + text +
  optional source_segment_id/at_ms/speaker_label), covering both origins the Decisions section
  names ("a marker or a manually-selected transcript excerpt becomes an ActionItem a user
  explicitly creates") with one message rather than two. `ActionItem` has no timestamp field, so
  — unlike `Track`/`Recording` — it's used directly as a bun model, the same flat-struct shortcut
  `crud`'s own `Name` takes (`protoc-gen-gotag` already stamped every field with a `bun:"..."` tag;
  table name is bun's own default pluralization, "ActionItem" → "action_items"). `CreateLinemanTasks`
  calls Lineman's real `CreateTask` once per action item (standalone, no `objective_id` — Relay has
  no notion of a Lineman Objective), fails the whole batch on the first Lineman error rather than
  attempting a partial-success result the wire shape has no way to report, and records each
  resulting task id back onto its action item. Live-verified against a real Lineman instance:
  two `ActionItem`s created (one from a real transcript segment excerpt, one marker-style with a
  timestamp+speaker label) → `ListActionItems` scoped to a recording and unscoped both correct →
  `CreateLinemanTasks` returned two real task ids → both fetched back via Lineman's own `GetTask`
  with correct name/state → both action items show `lineman_task_id` set afterward. One thing
  observed live, unrelated to Relay's own code: Lineman's storage is Blueprint's raft-replicated
  KV, which can show a just-committed write to an *immediate* follow-up read with a small lag under
  back-to-back writes — confirmed by retrying a failed `GetTask` a moment later and finding it;
  `CreateLinemanTasks` itself already had a valid id directly from `CreateTask`'s own response, so
  this never affected Relay's actual behavior, only a verification script's immediate re-check.)_

- [x] **7. Telemetry + Foundry.** Wide events per [Telemetry](#telemetry); `relay.transcript.segment`
  CloudEvent production per finalized segment; Foundry manifest publish/retract via
  `chassis.Effect`. Deliverable: a `SearchTranscripts` call queryable via Beacon's
  `SearchWideEvents`; a standalone `Consume` subscriber for `relay.transcript.segment` receives a
  live segment during a real recording (this is provable now that the fan-out bug is fixed, unlike
  when Lineman's own equivalent deliverable was written); Relay listed in Foundry's catalog while
  running. _(completed 2026-09-30 — `relay.transcript.segment` CloudEvent production itself was
  already built and live-verified in Phase 4 (`service/events.go`), since it was natural to wire
  up alongside segment finalization rather than deferred to here; this phase's own actual scope
  was the Foundry manifest effect plus filling in two Telemetry business attributes an earlier
  phase's own table was missing (`StartRecording`'s `input_device_id`, `Play`'s
  `output_device_id` — `StopRecording` intentionally left without one: `StopRecordingRequest`
  carries only `recording_id`, no field to read it from). `service/foundry.go` mirrors
  `services/tooling/lineman/foundry.go`'s publish/retract shape exactly, same reasoning (discovery
  only, no `StepExecutor`). Live-verified all three deliverables for real: (1) Foundry's own
  `List` RPC shows `relay@v1` alongside every other running plugin; (2) a real `SearchTranscripts`
  call's `WideEvent` found via Beacon's `SearchWideEvents` (filtered on `service_name`/`span_name`),
  with `business_attributes["query"]`/`["result_count"]` matching exactly; (3) a standalone
  Catalyst `Consume` subscriber (opened asynchronously *before* the recording started, using the
  same `openConsumerAsync` shape `services/core/catalyst/broker/fanout_test.go` established this
  session — a synchronous `Consume()` call blocks until the server's first `Send`, which can't
  happen before the event that triggers it exists) received a real live
  `tooling.relay.v1.TranscriptSegment` CloudEvent mid-recording, correct recording id/text/
  confidence, exactly the "provable now that the fan-out bug is fixed" claim this checklist item
  itself names.)_

- [x] **8. Speaker diarization (archive only — see [Speech-to-text
  pipeline](#speech-to-text-pipeline)).** Build the `pyannote.audio` HTTP wrapper and its sidecar
  deployment (native-dev/container-prod, same as `whisper.cpp`); a batch pass runs once per
  finalized `Recording` and relabels its stored segments — the live Record page from Phase 4 is
  unaffected and stays permanently unlabeled. Deliverable: a real two-person recording, once
  stopped, has its archived transcript correctly split into "Speaker 1"/"Speaker 2" segments,
  user-renameable via `RenameSpeaker`; the same recording's live view (while it was recording)
  correctly never showed this. _(completed 2026-09-30 — with one real, disclosed limit: this
  phase's own gated-model dependency (see [Speech-to-text
  pipeline](#speech-to-text-pipeline)'s HuggingFace caveat) means the *pretrained pipeline itself*
  was never actually exercised this session — a HuggingFace account + access token is a real,
  one-time setup step outside this repo that nothing here can perform on the user's behalf.
  Everything else is real and complete: `pyannote-wrapper/server.py` (Relay's first actual Python
  component, exactly as the plan calls for — no off-the-shelf server exists for pyannote.audio the
  way `whisper.cpp` ships one) implements the full `POST /diarize` contract, loads the pretrained
  pipeline lazily, and returns a clear `503` explaining the HF_TOKEN setup when it's absent, rather
  than an unhelpful crash — confirmed live: started the real wrapper (FastAPI/uvicorn, no
  HF_TOKEN), confirmed `/healthz` reports the unmet dependency accurately and `/diarize` returns a
  real, actionable `503`. `service/diarize.go` (`Diarizer` interface + HTTP client) and the
  `diarizeRecording` orchestration in `rpc.go` (maps pyannote's raw `SPEAKER_NN` labels to
  persisted, sequentially-numbered `Speaker`s in order of first appearance; relabels every
  already-persisted `TranscriptSegment` by largest time-overlap with a diarized span) are fully
  built and were live-verified end-to-end against a small fake stand-in server returning
  deterministic synthetic speaker turns (2 speakers, one transcript segment's overlap
  deliberately split 1500ms/1200ms between them to give the "largest overlap wins" logic a single
  verifiable right answer) — confirmed correct, then confirmed `RenameSpeaker` and `GetRecording`
  both correctly reflect a renamed speaker afterward. Also confirmed graceful degradation against
  the *real* (token-less) wrapper: `StopRecording` still succeeds, `Recording.speakers` comes back
  empty, and every segment's `speaker_id` stays "" — exactly Phase 4's own baseline behavior, not a
  broken or half-finished state. Not yet wired into `run-local.sh`/`run-local-watch.sh` (see
  `pyannote-wrapper/README.md`'s own note) — do that once `HF_TOKEN` is actually configured in this
  environment; until then a missing sidecar degrades exactly the same way an unreachable
  `whisper.cpp` would. Incidentally root-caused a real, previously-only-observed bug while doing
  this work: rapid successive `watchexec` restarts while Relay's own Phase 7 Foundry effect was in
  play left it permanently crash-looping on Foundry's `AlreadyExists`, the same class of bug
  already flagged for Lineman — see [[project-local-dev-stack]] for the full root cause (a failed
  `Effect()` setup calls `Fatal` before `Start()`'s shutdown path is ever reachable, so a stuck
  registration can never self-heal) and the manual-retract remedy used to unblock this session.)_

- [x] **9. Native device support (PortAudio).** `ListDevices` backed by real local hardware
  enumeration on instances that have it; multi-instance addressing per
  [Multi-instance device ownership](#multi-instance-device-ownership-the-hard-part); `Play`/
  `StartRecording` routed to the owning instance instead of assuming "the browser" as the only
  device. Deliverable: two Relay instances on two different machines, each exposing its own real
  mic/speakers; play a library track out of a real speaker on a machine other than the one serving
  the web client; record from a real USB mic the same way, matching the mockups' full device list
  (not just browser-streamed audio) end to end. _(completed 2026-09-30 for real hardware
  **listing** only — a deliberate scope decision, not a blocker, made explicit upfront rather than
  discovered partway through: `Play`/`StartRecording` routing to a real native `device_id` is a
  disclosed follow-up, since it needs a `Player` generalized to one per output device (the plan's
  own "one `PlayerState` per output device" decision, not yet true in code — there's still exactly
  one shared `Player`) plus a native capture pump feeding `RecordingSession.Ingest` from real mic
  bytes instead of `StreamAudioIn`, neither built here. What *is* real and live-verified: added
  `github.com/gordonklaus/portaudio` (cgo) behind an opt-in `portaudio` build tag —
  `go build -tags portaudio .` (PortAudio installed, e.g. `brew install portaudio` on macOS) — so
  every existing deployment's zero-cgo default build (Phases 1-8, and this dev stack's own live
  instance) is completely unaffected; the untagged build reports no native hardware
  (`devices_noop.go`). `ListDevices` (`rpc.go`) always merges the two virtual browser devices with
  whatever the real `DeviceLister` finds. Live-verified on this actual machine: a `-tags portaudio`
  build correctly enumerated 3 real devices (iPhone Microphone via Continuity, MacBook Pro
  Microphone, MacBook Pro Speakers) with correct kind/channels/sample rate, confirmed both via a
  small standalone `cmd/list-devices` CLI (useful on its own for checking a PortAudio install
  independent of the full service) and via the actual `ListDevices` RPC handler called directly;
  separately confirmed the live untagged instance's real `ListDevices` RPC (over the network)
  still returns exactly the two browser devices, unaffected. Found and fixed a real, previously
  invisible bug while merging both device sets into one list for the first time:
  `BrowserInputDeviceID` and `BrowserOutputDeviceID` had silently shared the literal string
  `"browser"` since Phase 3/4 (each was only ever used alone, never together, until now) — the Go
  compiler caught it as a duplicate switch case in a verification script; renamed to
  `"browser-in"`/`"browser-out"`, no stored data affected (neither was ever persisted to a
  database row). Also hit the Phase 8-documented Foundry `AlreadyExists` crash-loop again during
  this phase's own rapid edit cycle — see [[project-local-dev-stack]], same manual-retract remedy.
  "Two Relay instances on two different machines" couldn't be demonstrated for real (one dev
  machine available) — multi-instance *aggregation* is explicitly a caller-side concern per this
  section's own text (Query Blueprint for every `relay` instance, call `ListDevices` on each), not
  something this service's own code does to itself, so there was nothing further to build or
  simulate there beyond what's already verified.)_

- [ ] **10. Web client.** `services/tooling/relay/web-client`, Dioxus, implementing
  `relay-library.html`, `relay-record.html`, `relay-transcripts.html` against the real RPCs from
  Phases 3–9 (built incrementally alongside each phase in practice, listed once here for
  completeness). Deliverable: all three pages render against real data with no mock/simulated
  waveform, meter, or transcript data left in the shipped client. _(in progress as of 2026-09-30 —
  Library and Record are done and live-verified; Transcripts is not built yet. Scaffolded
  `services/tooling/relay/web-client` mirroring `services/tooling/lineman/web-client` exactly
  (Cargo.toml/Dioxus.toml/index.html/`.cargo/config.toml`), added `AppKind::Relay` to
  `tools/draft-ui` (no existing hue was free, so it takes `Tone::Primary` rather than overloading
  `Err`/`Warn`'s own semantics). **Library**: real `ListTracks`/search, real client-streaming
  `UploadTrack` (a real file input, `FileData::read_bytes()`, chunked into `UploadTrackChunk`s),
  real `Play`/`Pause`/`Seek`/`SetVolume`/`GetPlayerState` (polled every 750ms — the shared
  `PlayerState` could be driven by another tab), and *real playback*: `StreamAudioOut`'s raw
  PCM16LE chunks are scheduled through an actual Web Audio graph
  (`assets/audio-player.js`, driven via `document::eval` — hand-written JS for the same reason
  `draft-ui`'s own `boot.js` is, not web-sys typed-array juggling from Rust). No mock waveform: a
  plain seekable position slider stands in for the mockup's own peak-data rendering (a disclosed
  scope trim, not fake data) and the Output field is a fixed label, not a device-picker dropdown
  offering examples that don't route anywhere real (Play-to-a-real-native-device is Phase 9's own
  disclosed remainder). **A real, load-bearing gap found and fixed while building this**: the
  client has no way to decode `StreamAudioOut`'s raw PCM without knowing its sample rate/channel
  count, and `Track.format` is a display string that doesn't even state channel count — added
  structured `sample_rate_hz`/`channels`/`bits_per_sample` fields to `Track` (a live migration for
  the already-existing `tracks` table, `CreateSchema`). **Live-verified end-to-end in a real
  browser** (`dx serve` + Chrome): real track list rendering actual data from every earlier
  phase's own testing; a real 20-second WAV uploaded through the file input and confirmed correct
  in the list; clicking Play advances the position slider in real wall-clock time; Pause freezes
  it; Seek jumps to a new position and playback resumes correctly from there; a track finishing
  naturally resets to a stopped state exactly as the server's own `Finished()` does. Also hit the
  Phase 8/9-documented Foundry `AlreadyExists` crash-loop twice more during this phase's own edit
  cycle — same manual-retract remedy, see [[project-local-dev-stack]].

  **Record**: real `StartRecording`/`StopRecording`/`AddMarker`, and a live transcript via
  `WatchTranscript` (server-streaming works fine over grpc-web, unlike client-streaming — see
  below). **A second real, load-bearing gap found and fixed while building this**: reading
  `tonic-web-wasm-client`'s own transport source (`call.rs::prepare_body`) confirmed it collects an
  entire client-streaming request into one `fetch()` body before sending anything — so the existing
  `StreamAudioIn` client-streaming RPC (built in Phase 4 for exactly this) can never deliver a live,
  open-ended mic recording incrementally from a browser; it would only ever reach the server after
  the whole call closed. Fixed by adding a new unary RPC, `StreamAudioInChunk`, called once per
  captured audio buffer instead — it reuses the exact same `RecordingSession.Ingest` path
  `StreamAudioIn`'s own receive loop uses, just one chunk per call. Real mic capture: a single
  inline `document::eval` running `getUserMedia`/`AudioContext`/`ScriptProcessorNode` (not
  `AudioWorkletNode` — no separate module file needed), converting each buffer to PCM16LE and
  sending it base64-encoded back to Rust via `dioxus.send()` alongside its own peak dBFS, driving a
  real `Sparkline` level history — not a canvas animation standing in for one. A silent `GainNode`
  (gain 0) keeps the audio graph "active" without looping the mic back to the speakers.
  **Live-verified end-to-end in a real browser**: clicking Start fires `StartRecording`, grants a
  real mic-permission prompt (Chrome's own, answered by the user — not something automation should
  click through), and begins real capture; the Sparkline and dBFS reading track actual mic input in
  real time; dozens of real `StreamAudioInChunk` calls land at the server (all `200`) and come back
  out the other side as real live transcript segments from the already-live whisper.cpp pipeline
  (verbatim captured speech, including a `[BLANK_AUDIO]` segment during a pause — not canned text);
  `AddMarker` succeeds and increments the marker count live; word count tracks the real transcript;
  Stop finalizes the recording and the UI cleanly reverts to its idle state. One real operational
  gotcha hit along the way: the server enforces a single active recording at a time
  (`RecordingSession.Active()`), so a recording abandoned by navigating away mid-session (rather
  than clicking Stop) leaves a stale "recording already in progress" state blocking the next
  `StartRecording` call until it's explicitly stopped — not a bug, just something to know if a
  dev-loop reload interrupts an in-progress recording.)_

- [ ] **11. Verification.** Live, end-to-end: upload and play a library track from the browser;
  record a real conversation, watch live captions appear, stop and confirm it's searchable;
  search across multiple recordings and confirm correct hits/excerpts; promote action items into
  real Lineman tasks and confirm they appear on Lineman's board; confirm `relay.transcript.segment`
  events are independently visible via a `Consume` subscriber; confirm Relay is listed in
  Foundry's catalog while running; (once Phase 9 lands) confirm playback/recording against real,
  non-browser hardware on a second node.

## Open Questions

None remaining as of 2026-09-30 — every question raised while drafting this plan was reviewed and
resolved (see [Decisions](#decisions)) before any phase started. Add new ones here as
implementation surfaces them; don't leave this section stale once it isn't empty anymore.

## Milestone Summary

| Phase | Scope | Status |
|---|---|---|
| 1 | Scaffolding | Done |
| 2 | Proto: `models.proto`, `service.proto` | Done |
| 3 | Library + browser playback | Done |
| 4 | Recording + live transcription (browser audio) | Done |
| 5 | Archive + full-text search | Done |
| 6 | Action items → Lineman | Done |
| 7 | Telemetry + Foundry | Done |
| 8 | Speaker diarization | Done (pending user's own HuggingFace token setup for the real model) |
| 9 | Native device support (PortAudio, multi-instance) | Done (device listing only; Play/StartRecording-to-real-device routing deferred) |
| 10 | Web client (all three mockup pages) | In progress — Library and Record done, Transcripts pending |
| 11 | Verification | Not started |
