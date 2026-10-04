// Command relay is the audio service described in
// docs/website/content/docs/architecture/relay-implementation-plan.md: one service that owns
// every audio input/output device in the cluster, plays and records audio, transcribes what it
// captures, and turns action items into Lineman tasks.
//
// Phases 1-3 (Scaffolding, Proto, Library) are in place: track metadata plus playback against
// browser-streamed/uploaded audio only. Phase 4 (Recording + live transcription) adds
// StartRecording/StreamAudioIn/AddMarker/StopRecording plus WatchTranscript for live captions,
// backed by a whisper.cpp sidecar (see service/transcribe.go) -- every segment shows a single
// unlabeled speaker while live; Phase 8 (below) relabels them retroactively once the recording is
// archived, never live. Phase 5 adds ListRecordings/GetRecording/GetTranscript/SearchTranscripts, the
// latter backed by real Postgres full-text search. Phase 6 adds CreateActionItem/ListActionItems/
// CreateLinemanTasks, the last of which calls Lineman's own real CreateTask RPC. Phase 7 adds a
// Foundry catalog entry (discovery only, no StepExecutor -- same as Lineman's own decision) and
// fills in the remaining Telemetry business attributes the plan's own table names. Everything
// else RelayService declares (devices, queue) answers CodeUnimplemented via service.Handler's
// embedded UnimplementedRelayServiceHandler until its own phase fills it in. Phase 8 adds batch
// speaker diarization (archive only, permanently -- see service/diarize.go): StopRecording runs a
// pyannote.audio sidecar pass over the finalized recording and relabels its stored segments; the
// live view from Phase 4 stays permanently unlabeled by design. Phase 9 adds real native hardware
// enumeration (service/devices*.go) -- opt-in via the "portaudio" build tag (see devices.go);
// listing only, Play/StartRecording against a real device_id remains a disclosed follow-up.
package main

import (
	"context"
	"embed"
	"net/http"
	"os"

	linemanv1Connect "github.com/steady-bytes/draft/api/tooling/lineman/v1/v1connect"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"
	"github.com/steady-bytes/draft/services/tooling/relay/service"
)

// Phase 10's web-client never had a UI route registered with Fuse until now -- same
// WithRoute+WithClientApplication shape Beacon/Lineman/Bench/Foundry's own main.go already use, so
// relay.draft.localhost actually serves the Dioxus build and shows up in every other app's
// discovery-driven Apps nav (tools/draft-ui/src/shell/discovery.rs), not just via AppKind::Relay's
// own styling (which was already correct -- it just had nothing to discover).
//
//go:embed web-client/target/dx/relay-pwa/release/web/public
var files embed.FS

const (
	defaultStorageDir   = "./.relay-storage"
	defaultWhisperAddr  = "http://127.0.0.1:9309"
	defaultLinemanAddr  = "http://localhost:9307"
	defaultFoundryAddr  = "http://localhost:9301"
	defaultPyannoteAddr = "http://127.0.0.1:9310"
)

func main() {
	logger := chassis.NewOTelLogger()
	chassis.NewMetricsReporter().Start()

	db := bun.New("")
	cfg := chassis.GetConfig()

	storageDir := cfg.GetString("relay.storage_dir")
	if storageDir == "" {
		storageDir = defaultStorageDir
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		logger.WithError(err).WithField("storage_dir", storageDir).Fatal("failed to create relay storage_dir")
	}

	// storage_node identifies which instance's local disk holds a Track's bytes (see
	// api/tooling/relay/v1/models.proto's Track.storage_node doc). Phase 3 is single-instance, so
	// any stable value would do; the real hostname is used anyway so it's already meaningful once
	// Phase 9 makes multi-instance device ownership real, rather than a placeholder to revisit.
	storageNode, err := os.Hostname()
	if err != nil || storageNode == "" {
		storageNode = "relay"
	}

	whisperAddr := cfg.GetString("relay.whisper.address")
	if whisperAddr == "" {
		whisperAddr = defaultWhisperAddr
	}

	catalystAddr := cfg.GetString("catalyst.address")
	if catalystAddr == "" {
		catalystAddr = "http://localhost:2220"
	}

	linemanAddr := cfg.GetString("relay.lineman.address")
	if linemanAddr == "" {
		linemanAddr = defaultLinemanAddr
	}
	linemanClient := linemanv1Connect.NewLinemanServiceClient(http.DefaultClient, linemanAddr)

	foundryAddr := cfg.GetString("foundry.address")
	if foundryAddr == "" {
		foundryAddr = defaultFoundryAddr
	}
	foundryClient := service.NewFoundryClient(foundryAddr)
	manifest := service.BuildManifest()

	pyannoteAddr := cfg.GetString("relay.pyannote.address")
	if pyannoteAddr == "" {
		pyannoteAddr = defaultPyannoteAddr
	}
	diarizer := service.NewPyannoteDiarizer(pyannoteAddr)

	deviceLister, err := service.NewDeviceLister()
	if err != nil {
		logger.WithError(err).Fatal("failed to initialize device lister")
	}

	// eventsCtx lives for the whole process (cancelled on chassis.Closer()), not scoped to any one
	// request -- the events publisher's stream to Catalyst is opened once and reused by every
	// finalized transcript segment for as long as this service runs. Same pattern
	// services/examples/crud-event/main.go's own eventsCtx follows.
	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	go func() {
		<-chassis.Closer()
		cancelEvents()
	}()

	model := service.NewModel(db, storageDir, storageNode)
	player := service.NewPlayer()
	recordings := service.NewRecordingModel(db)
	sessions := service.NewSessionManager()
	transcriber := service.NewWhisperTranscriber(whisperAddr)
	events := service.NewEventPublisher(eventsCtx, logger, catalystAddr, "/services/relay")

	rpcHandler := service.NewHandler(logger, model, player, storageDir, recordings, sessions, transcriber, events, linemanClient, diarizer, deviceLister)

	runtime := chassis.New(logger).
		Register(chassis.RegistrationOptions{
			Namespace: "tooling",
		}).
		WithRepository(db).
		WithRPCHandler(rpcHandler).
		// Publishing to Foundry's catalog is the "ad hoc effect with an inverse" shape
		// chassis.Effect exists for -- publish on startup, retract on graceful shutdown. Relay
		// does not implement StepExecutor; this manifest exists purely for discovery/distribution,
		// the same reasoning/precedent as Lineman's own decision.
		Effect("foundry-catalog-entry", service.FoundryCatalogEffectSetup(logger, foundryClient, manifest)).
		// Releases whatever process-wide audio resources NewDeviceLister acquired (PortAudio's
		// Initialize/Terminate pairing under the "portaudio" build tag; a no-op otherwise) on
		// graceful shutdown -- no setup step needed here since construction already happened above.
		Effect("device-lister-cleanup", func() (func(context.Context) error, error) {
			return func(ctx context.Context) error { return deviceLister.Close() }, nil
		}).
		WithRoute(&ntv1.Route{
			Name: "relay-rpc",
			Match: &ntv1.RouteMatch{
				Prefix: "/tooling.relay.v1.RelayService/",
			},
		}).
		WithRoute(&ntv1.Route{
			Name: "relay-ui",
			Match: &ntv1.RouteMatch{
				Host:   "relay.draft.localhost",
				Prefix: "/",
			},
			EnableHttp2: true,
		}).
		WithClientApplication(files, "web-client/target/dx/relay-pwa/release/web/public")

	if err := service.CreateSchema(context.Background(), db); err != nil {
		logger.WithError(err).Fatal("failed to create relay tracks schema")
	}
	if err := service.CreateRecordingSchema(context.Background(), db); err != nil {
		logger.WithError(err).Fatal("failed to create relay recordings schema")
	}

	defer runtime.Start()
}
