package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	bunorm "github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// recordingRow, transcriptSegmentRow, and markerRow are Relay's storage shapes for Recording/
// TranscriptSegment/Marker. Recording carries two google.protobuf.Timestamp fields
// (started_at/finished_at) -- the same reasoning as trackRow in model.go applies: a dedicated row
// + conversion functions, not the proto type used directly as a bun model (see
// services/tooling/bench/model.go's own precedent for this exact case). TranscriptSegment and
// Marker are flat enough that this isn't strictly required for them, but sharing the same
// row/toProto/fromProto shape as everything else in this package keeps the three consistent
// rather than having tracks be the odd one out.
//
// speakers is deliberately absent from recordingRow: diarization is batch-only and lands in
// Phase 8 (see the plan's Speech-to-text pipeline section) -- Recording.speakers is always empty
// until then, and there is no speakers table yet.
type recordingRow struct {
	bunorm.BaseModel `bun:"table:recordings,alias:r"`

	Id         string     `bun:"id,pk"`
	Name       string     `bun:"name,notnull,default:''"`
	TrackId    string     `bun:"track_id,notnull,default:''"`
	Status     int32      `bun:"status,notnull,default:0"` // relayv1.RecordingStatus
	DurationMs int64      `bun:"duration_ms,notnull,default:0"`
	WordCount  int32      `bun:"word_count,notnull,default:0"`
	StartedAt  time.Time  `bun:"started_at,nullzero,notnull,default:current_timestamp"`
	FinishedAt *time.Time `bun:"finished_at"` // nil while still recording
}

func (r *recordingRow) toProto() *relayv1.Recording {
	rec := &relayv1.Recording{
		Id:         r.Id,
		Name:       r.Name,
		TrackId:    r.TrackId,
		Status:     relayv1.RecordingStatus(r.Status),
		DurationMs: r.DurationMs,
		WordCount:  r.WordCount,
		StartedAt:  timestamppb.New(r.StartedAt),
	}
	if r.FinishedAt != nil {
		rec.FinishedAt = timestamppb.New(*r.FinishedAt)
	}
	return rec
}

type transcriptSegmentRow struct {
	bunorm.BaseModel `bun:"table:transcript_segments,alias:ts"`

	Id          string  `bun:"id,pk"`
	RecordingId string  `bun:"recording_id,notnull"`
	StartMs     int64   `bun:"start_ms,notnull"`
	EndMs       int64   `bun:"end_ms,notnull"`
	SpeakerId   string  `bun:"speaker_id,notnull,default:''"` // always empty until Phase 8
	Text        string  `bun:"text,notnull,default:''"`
	Confidence  float32 `bun:"confidence,notnull,default:0"`
}

func (r *transcriptSegmentRow) toProto() *relayv1.TranscriptSegment {
	return &relayv1.TranscriptSegment{
		Id:          r.Id,
		RecordingId: r.RecordingId,
		StartMs:     r.StartMs,
		EndMs:       r.EndMs,
		SpeakerId:   r.SpeakerId,
		Text:        r.Text,
		Confidence:  r.Confidence,
		// is_partial is always false here: only finalized segments are ever persisted (see
		// session.go) -- there is no partial-segment row to represent.
		IsPartial: false,
	}
}

type markerRow struct {
	bunorm.BaseModel `bun:"table:markers,alias:mk"`

	Id          string `bun:"id,pk"`
	RecordingId string `bun:"recording_id,notnull"`
	AtMs        int64  `bun:"at_ms,notnull"`
	Label       string `bun:"label,notnull,default:''"`
}

func (r *markerRow) toProto() *relayv1.Marker {
	return &relayv1.Marker{Id: r.Id, RecordingId: r.RecordingId, AtMs: r.AtMs, Label: r.Label}
}

// speakerRow is Phase 8's storage shape for a Speaker. The wire message (models.proto) is just
// {id, label} -- no recording_id -- but the row needs one to answer "which recording's speakers
// are these" (GetSpeakers) without it ever appearing on the wire; toProto drops it accordingly.
type speakerRow struct {
	bunorm.BaseModel `bun:"table:speakers,alias:sp"`

	Id          string `bun:"id,pk"`
	RecordingId string `bun:"recording_id,notnull"`
	Label       string `bun:"label,notnull,default:''"`
}

func (r *speakerRow) toProto() *relayv1.Speaker {
	return &relayv1.Speaker{Id: r.Id, Label: r.Label}
}

// CreateRecordingSchema creates the recordings/transcript_segments/markers tables if they don't
// already exist. Separate from CreateSchema (model.go's tracks table) only for readability --
// both are called together from main.go.
func CreateRecordingSchema(ctx context.Context, db bun.Repository) error {
	for _, model := range []any{(*recordingRow)(nil), (*transcriptSegmentRow)(nil), (*markerRow)(nil), (*speakerRow)(nil)} {
		if _, err := db.Client().NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			return fmt.Errorf("failed to create table for %T: %w", model, err)
		}
	}

	// Full-text search over transcript text (SearchTranscripts, Phase 5) -- a generated tsvector
	// column + GIN index, deliberately not a transcriptSegmentRow Go field: bun's model only needs
	// to declare columns it reads/writes directly through struct fields, and this one is computed
	// and indexed entirely inside Postgres (SearchTranscripts below references it by name in a raw
	// Where() fragment, which works regardless of whether the Go struct knows about it). This is
	// exactly the capability the plan's "Postgres, not Blueprint KV" decision was written for --
	// Blueprint's raft-replicated KV store has no query capability beyond key lookup.
	if _, err := db.Client().NewRaw(`
		ALTER TABLE transcript_segments
		ADD COLUMN IF NOT EXISTS text_search tsvector GENERATED ALWAYS AS (to_tsvector('english', text)) STORED
	`).Exec(ctx); err != nil {
		return fmt.Errorf("failed to add transcript_segments.text_search: %w", err)
	}
	if _, err := db.Client().NewRaw(`
		CREATE INDEX IF NOT EXISTS transcript_segments_text_search_idx
		ON transcript_segments USING GIN (text_search)
	`).Exec(ctx); err != nil {
		return fmt.Errorf("failed to create transcript_segments_text_search_idx: %w", err)
	}

	// ActionItem (Phase 6) has no timestamp field at all -- flat enough to use the generated proto
	// struct directly as a bun model, the same shortcut services/examples/crud's own Name takes
	// (protoc-gen-gotag already stamped every field with a bun tag; no dedicated row type needed
	// the way trackRow/recordingRow are for their timestamp fields). Table name is bun's own
	// default pluralization of the Go type name ("ActionItem" -> "action_items"), confirmed
	// against the same convention crud's "Name" -> "names" already relies on.
	if _, err := db.Client().NewCreateTable().Model((*relayv1.ActionItem)(nil)).IfNotExists().Exec(ctx); err != nil {
		return fmt.Errorf("failed to create table for %T: %w", (*relayv1.ActionItem)(nil), err)
	}
	return nil
}

type (
	// RecordingModel is Relay's Phase 4 persistence for recordings, their finalized transcript
	// segments, and user-added markers. Kept separate from Model (tracks) the same way rpc.go's
	// handler composes several small pieces rather than one do-everything interface, but backed
	// by the same bun.Repository.
	RecordingModel interface {
		StartRecording(ctx context.Context, name string) (*relayv1.Recording, error)
		// FinishRecording updates a recording's terminal state once StopRecording has finalized
		// its Track and computed real duration/word-count/confidence -- trackID/status/durationMs/
		// wordCount all come from session.go's own bookkeeping, not recomputed here.
		FinishRecording(ctx context.Context, id, trackID string, status relayv1.RecordingStatus, durationMs int64, wordCount int32) (*relayv1.Recording, error)
		InsertTranscriptSegment(ctx context.Context, recordingID string, seg TranscriptSegment) (*relayv1.TranscriptSegment, error)
		AddMarker(ctx context.Context, recordingID string, atMs int64, label string) (*relayv1.Marker, error)

		// Phase 5 (Archive + search).
		ListRecordings(ctx context.Context) ([]*relayv1.Recording, error)
		GetRecording(ctx context.Context, id string) (*relayv1.Recording, error)
		GetTranscript(ctx context.Context, recordingID string) ([]*relayv1.TranscriptSegment, error)
		SearchTranscripts(ctx context.Context, query string) ([]*relayv1.SearchHit, error)

		// Phase 6 (Action items -> Lineman).
		CreateActionItem(ctx context.Context, req *relayv1.CreateActionItemRequest) (*relayv1.ActionItem, error)
		// ListActionItems: an empty recordingID matches every recording (per the .proto doc).
		ListActionItems(ctx context.Context, recordingID string) ([]*relayv1.ActionItem, error)
		GetActionItems(ctx context.Context, ids []string) ([]*relayv1.ActionItem, error)
		SetLinemanTaskID(ctx context.Context, actionItemID, linemanTaskID string) error

		// Phase 8 (Speaker diarization -- archive only, see diarize.go).
		GetSpeakers(ctx context.Context, recordingID string) ([]*relayv1.Speaker, error)
		CreateSpeaker(ctx context.Context, recordingID, label string) (*relayv1.Speaker, error)
		RenameSpeaker(ctx context.Context, speakerID, label string) (*relayv1.Speaker, error)
		SetTranscriptSegmentSpeaker(ctx context.Context, segmentID, speakerID string) error
	}
	recordingModel struct {
		db bun.Repository
	}
)

func NewRecordingModel(db bun.Repository) RecordingModel {
	return &recordingModel{db: db}
}

func (m *recordingModel) StartRecording(ctx context.Context, name string) (*relayv1.Recording, error) {
	row := &recordingRow{
		Id:        uuid.New().String(),
		Name:      name,
		Status:    int32(relayv1.RecordingStatus_RECORDING_STATUS_LIVE),
		StartedAt: time.Now(),
	}
	if _, err := m.db.Client().NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to insert recording: %w", err)
	}
	return row.toProto(), nil
}

func (m *recordingModel) FinishRecording(ctx context.Context, id, trackID string, status relayv1.RecordingStatus, durationMs int64, wordCount int32) (*relayv1.Recording, error) {
	row := &recordingRow{}
	_, err := m.db.Client().NewUpdate().Model(row).
		Set("track_id = ?", trackID).
		Set("status = ?", int32(status)).
		Set("duration_ms = ?", durationMs).
		Set("word_count = ?", wordCount).
		Set("finished_at = ?", time.Now()).
		Where("id = ?", id).
		Returning("*").
		Exec(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("failed to finalize recording %q: %w", id, err)
	}
	return row.toProto(), nil
}

func (m *recordingModel) InsertTranscriptSegment(ctx context.Context, recordingID string, seg TranscriptSegment) (*relayv1.TranscriptSegment, error) {
	row := &transcriptSegmentRow{
		Id:          uuid.New().String(),
		RecordingId: recordingID,
		StartMs:     seg.StartMs,
		EndMs:       seg.EndMs,
		Text:        seg.Text,
		Confidence:  seg.Confidence,
	}
	if _, err := m.db.Client().NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to insert transcript segment: %w", err)
	}
	return row.toProto(), nil
}

func (m *recordingModel) AddMarker(ctx context.Context, recordingID string, atMs int64, label string) (*relayv1.Marker, error) {
	row := &markerRow{
		Id:          uuid.New().String(),
		RecordingId: recordingID,
		AtMs:        atMs,
		Label:       label,
	}
	if _, err := m.db.Client().NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to insert marker: %w", err)
	}
	return row.toProto(), nil
}

func (m *recordingModel) ListRecordings(ctx context.Context) ([]*relayv1.Recording, error) {
	var rows []*recordingRow
	if err := m.db.Client().NewSelect().Model(&rows).OrderExpr("started_at DESC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list recordings: %w", err)
	}
	recs := make([]*relayv1.Recording, len(rows))
	for i, row := range rows {
		rec := row.toProto()
		// One query per recording for its speakers (Phase 8) -- an N+1, acceptable at this scale
		// (a personal archive, not a high-traffic listing); not worth a batched join for the
		// number of recordings this is ever likely to hold.
		speakers, err := m.GetSpeakers(ctx, row.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to load speakers for recording %q: %w", row.Id, err)
		}
		rec.Speakers = speakers
		recs[i] = rec
	}
	return recs, nil
}

func (m *recordingModel) GetRecording(ctx context.Context, id string) (*relayv1.Recording, error) {
	row := &recordingRow{}
	if err := m.db.Client().NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("recording %q: %w", id, err)
	}
	rec := row.toProto()
	speakers, err := m.GetSpeakers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load speakers for recording %q: %w", id, err)
	}
	rec.Speakers = speakers
	return rec, nil
}

func (m *recordingModel) GetTranscript(ctx context.Context, recordingID string) ([]*relayv1.TranscriptSegment, error) {
	var rows []*transcriptSegmentRow
	err := m.db.Client().NewSelect().Model(&rows).
		Where("recording_id = ?", recordingID).
		OrderExpr("start_ms ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get transcript for recording %q: %w", recordingID, err)
	}
	segs := make([]*relayv1.TranscriptSegment, len(rows))
	for i, row := range rows {
		segs[i] = row.toProto()
	}
	return segs, nil
}

// SearchTranscripts runs a Postgres full-text query over every transcript_segments.text (see
// CreateRecordingSchema's text_search column/index above) and groups the matching segments by
// recording, so each hit carries the Recording it belongs to alongside just the segments that
// actually matched -- mirrors relay-transcripts.html's own list ("4 hits" per row) plus its
// drawer's excerpt list without a second round trip per hit (see SearchHit's own .proto doc).
func (m *recordingModel) SearchTranscripts(ctx context.Context, query string) ([]*relayv1.SearchHit, error) {
	var segRows []*transcriptSegmentRow
	err := m.db.Client().NewSelect().Model(&segRows).
		Where("text_search @@ plainto_tsquery('english', ?)", query).
		OrderExpr("recording_id, start_ms").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to search transcripts: %w", err)
	}
	if len(segRows) == 0 {
		return nil, nil
	}

	// segRows is already ordered by recording_id, so a matched recording's rows are always
	// contiguous -- this both groups them and preserves a stable, sensible hit order (most
	// recently touched by the query plan's own scan, not re-sorted by relevance -- Phase 5 has no
	// ranking beyond "matched or didn't").
	var order []string
	byRecording := make(map[string][]*relayv1.TranscriptSegment)
	for _, row := range segRows {
		if _, seen := byRecording[row.RecordingId]; !seen {
			order = append(order, row.RecordingId)
		}
		byRecording[row.RecordingId] = append(byRecording[row.RecordingId], row.toProto())
	}

	var recordingRows []*recordingRow
	if err := m.db.Client().NewSelect().Model(&recordingRows).Where("id IN (?)", bunorm.In(order)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to load matched recordings: %w", err)
	}
	recordingsByID := make(map[string]*recordingRow, len(recordingRows))
	for _, r := range recordingRows {
		recordingsByID[r.Id] = r
	}

	hits := make([]*relayv1.SearchHit, 0, len(order))
	for _, recID := range order {
		rec, ok := recordingsByID[recID]
		if !ok {
			// The recording was deleted out from under an already-indexed segment -- skip rather
			// than error; DeleteRecording isn't implemented yet (see rpc.go), so this shouldn't
			// currently be reachable, but there's no reason SearchTranscripts should ever hard-fail
			// because of it once it is.
			continue
		}
		hits = append(hits, &relayv1.SearchHit{Recording: rec.toProto(), Matches: byRecording[recID]})
	}
	return hits, nil
}

func (m *recordingModel) CreateActionItem(ctx context.Context, req *relayv1.CreateActionItemRequest) (*relayv1.ActionItem, error) {
	item := &relayv1.ActionItem{
		Id:              uuid.New().String(),
		RecordingId:     req.GetRecordingId(),
		Text:            req.GetText(),
		SourceSegmentId: req.GetSourceSegmentId(),
		AtMs:            req.GetAtMs(),
		SpeakerLabel:    req.GetSpeakerLabel(),
	}
	if _, err := m.db.Client().NewInsert().Model(item).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to insert action item: %w", err)
	}
	return item, nil
}

func (m *recordingModel) ListActionItems(ctx context.Context, recordingID string) ([]*relayv1.ActionItem, error) {
	var items []*relayv1.ActionItem
	query := m.db.Client().NewSelect().Model(&items).OrderExpr("at_ms ASC")
	if recordingID != "" {
		query = query.Where("recording_id = ?", recordingID)
	}
	if err := query.Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list action items: %w", err)
	}
	return items, nil
}

func (m *recordingModel) GetActionItems(ctx context.Context, ids []string) ([]*relayv1.ActionItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var items []*relayv1.ActionItem
	if err := m.db.Client().NewSelect().Model(&items).Where("id IN (?)", bunorm.In(ids)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to load action items: %w", err)
	}
	return items, nil
}

func (m *recordingModel) SetLinemanTaskID(ctx context.Context, actionItemID, linemanTaskID string) error {
	_, err := m.db.Client().NewUpdate().Model((*relayv1.ActionItem)(nil)).
		Set("lineman_task_id = ?", linemanTaskID).
		Where("id = ?", actionItemID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set lineman_task_id on action item %q: %w", actionItemID, err)
	}
	return nil
}

func (m *recordingModel) GetSpeakers(ctx context.Context, recordingID string) ([]*relayv1.Speaker, error) {
	var rows []*speakerRow
	if err := m.db.Client().NewSelect().Model(&rows).Where("recording_id = ?", recordingID).OrderExpr("label ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to load speakers for recording %q: %w", recordingID, err)
	}
	speakers := make([]*relayv1.Speaker, len(rows))
	for i, row := range rows {
		speakers[i] = row.toProto()
	}
	return speakers, nil
}

func (m *recordingModel) CreateSpeaker(ctx context.Context, recordingID, label string) (*relayv1.Speaker, error) {
	row := &speakerRow{Id: uuid.New().String(), RecordingId: recordingID, Label: label}
	if _, err := m.db.Client().NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to insert speaker: %w", err)
	}
	return row.toProto(), nil
}

func (m *recordingModel) RenameSpeaker(ctx context.Context, speakerID, label string) (*relayv1.Speaker, error) {
	row := &speakerRow{}
	_, err := m.db.Client().NewUpdate().Model(row).
		Set("label = ?", label).
		Where("id = ?", speakerID).
		Returning("*").
		Exec(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("failed to rename speaker %q: %w", speakerID, err)
	}
	return row.toProto(), nil
}

func (m *recordingModel) SetTranscriptSegmentSpeaker(ctx context.Context, segmentID, speakerID string) error {
	_, err := m.db.Client().NewUpdate().Model((*transcriptSegmentRow)(nil)).
		Set("speaker_id = ?", speakerID).
		Where("id = ?", segmentID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set speaker_id on transcript segment %q: %w", segmentID, err)
	}
	return nil
}
