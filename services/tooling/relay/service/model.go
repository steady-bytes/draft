package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	relayv1 "github.com/steady-bytes/draft/api/tooling/relay/v1"
	"github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	bunorm "github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// trackRow is Relay's storage shape for a Track. Track carries a google.protobuf.Timestamp
// (added_at), a nested message with no native Postgres column mapping through bun -- the same
// reasoning as services/tooling/bench/model.go's workflowRow, not services/examples/crud's flat
// case (crudv1.Name has no timestamp field, so it's used directly as a bun model there). AddedAt
// is stored as time.Time and converted at the boundary by trackRowFromProto/toProto.
type trackRow struct {
	bunorm.BaseModel `bun:"table:tracks,alias:t"`

	Id            string    `bun:"id,pk"`
	Title         string    `bun:"title,notnull,default:''"`
	Artist        string    `bun:"artist,notnull,default:''"`
	Album         string    `bun:"album,notnull,default:''"`
	Format        string    `bun:"format,notnull,default:''"`
	DurationMs    int64     `bun:"duration_ms,notnull,default:0"`
	SizeBytes     int64     `bun:"size_bytes,notnull,default:0"`
	StorageNode   string    `bun:"storage_node,notnull,default:''"`
	StoragePath   string    `bun:"storage_path,notnull,default:''"`
	AddedAt       time.Time `bun:"added_at,nullzero,notnull,default:current_timestamp"`
	RecordingId   string    `bun:"recording_id,notnull,default:''"`
	SampleRateHz  int32     `bun:"sample_rate_hz,notnull,default:0"`
	Channels      int32     `bun:"channels,notnull,default:0"`
	BitsPerSample int32     `bun:"bits_per_sample,notnull,default:0"`
}

func (r *trackRow) toProto() *relayv1.Track {
	return &relayv1.Track{
		Id:            r.Id,
		Title:         r.Title,
		Artist:        r.Artist,
		Album:         r.Album,
		Format:        r.Format,
		DurationMs:    r.DurationMs,
		SizeBytes:     r.SizeBytes,
		StorageNode:   r.StorageNode,
		StoragePath:   r.StoragePath,
		AddedAt:       timestamppb.New(r.AddedAt),
		RecordingId:   r.RecordingId,
		SampleRateHz:  r.SampleRateHz,
		Channels:      r.Channels,
		BitsPerSample: r.BitsPerSample,
	}
}

func trackRowFromProto(t *relayv1.Track) *trackRow {
	addedAt := time.Now()
	if t.GetAddedAt() != nil {
		addedAt = t.GetAddedAt().AsTime()
	}
	return &trackRow{
		Id:            t.GetId(),
		Title:         t.GetTitle(),
		Artist:        t.GetArtist(),
		Album:         t.GetAlbum(),
		Format:        t.GetFormat(),
		DurationMs:    t.GetDurationMs(),
		SizeBytes:     t.GetSizeBytes(),
		StorageNode:   t.GetStorageNode(),
		StoragePath:   t.GetStoragePath(),
		AddedAt:       addedAt,
		RecordingId:   t.GetRecordingId(),
		SampleRateHz:  t.GetSampleRateHz(),
		Channels:      t.GetChannels(),
		BitsPerSample: t.GetBitsPerSample(),
	}
}

// CreateSchema creates the tracks table if it doesn't already exist. Mirrors
// services/examples/crud-event/service/model.go's CreateSchema.
func CreateSchema(ctx context.Context, db bun.Repository) error {
	if _, err := db.Client().NewCreateTable().Model((*trackRow)(nil)).IfNotExists().Exec(ctx); err != nil {
		return fmt.Errorf("failed to create table for %T: %w", (*trackRow)(nil), err)
	}
	// sample_rate_hz/channels/bits_per_sample were added in Phase 10, after `tracks` already
	// existed live -- IfNotExists above is a no-op against an existing table, so a real migration
	// step is needed for anyone who already ran Phase 3-9 against this database (this session's
	// own dev stack included).
	for _, column := range []string{"sample_rate_hz", "channels", "bits_per_sample"} {
		if _, err := db.Client().NewRaw(
			fmt.Sprintf(`ALTER TABLE tracks ADD COLUMN IF NOT EXISTS %s integer NOT NULL DEFAULT 0`, column),
		).Exec(ctx); err != nil {
			return fmt.Errorf("failed to add tracks.%s: %w", column, err)
		}
	}
	return nil
}

type (
	// Model is Relay's Phase 3 library: track metadata plus the local-disk file each one's bytes
	// live under. storage_node/storage_path are stamped by this same instance for now (Phase 3 is
	// single-instance); multi-instance device ownership is Phase 9.
	Model interface {
		ListTracks(ctx context.Context, filter string) ([]*relayv1.Track, error)
		GetTrack(ctx context.Context, id string) (*relayv1.Track, error)
		// AddTrack registers a metadata-only Track with no backing file -- UploadTrack (below) is
		// the primary Phase 3 flow ("upload a file through the web client"); this exists because
		// the interface declares it as its own RPC, independent of an upload.
		AddTrack(ctx context.Context, meta *relayv1.AddTrackRequest) (*relayv1.Track, error)
		// SaveUploadedTrack finishes an UploadTrack call (recordingID == "") or a StopRecording
		// finalization (recordingID set -- see session.go's Finalize): tmpPath is a fully-written
		// temp file (already under this Model's storage dir) holding real WAV bytes, either
		// streamed in as an UploadTrack call's chunks arrived or produced by finalizing a
		// recording's raw PCM. This parses it as a WAV file, moves it into its permanent location,
		// and inserts the row.
		SaveUploadedTrack(ctx context.Context, tmpPath string, meta *relayv1.AddTrackRequest, recordingID string) (*relayv1.Track, error)
		// DeleteTrack removes both the row and its backing file (best-effort on the file -- a
		// missing file on disk, eg. already cleaned up by hand, is not an error; a failing DB
		// delete is). Does not touch a Recording row even when this track is one's finalized
		// output -- see the .proto's own doc on DeleteTrack for why that's DeleteRecording's job.
		DeleteTrack(ctx context.Context, id string) error
	}
	model struct {
		db          bun.Repository
		storageDir  string
		storageNode string
	}
)

func NewModel(db bun.Repository, storageDir, storageNode string) Model {
	return &model{db: db, storageDir: storageDir, storageNode: storageNode}
}

func (m *model) ListTracks(ctx context.Context, filter string) ([]*relayv1.Track, error) {
	var rows []*trackRow
	query := m.db.Client().NewSelect().Model(&rows).OrderExpr("added_at DESC")
	if filter != "" {
		like := "%" + filter + "%"
		query = query.Where("title ILIKE ? OR artist ILIKE ? OR album ILIKE ?", like, like, like)
	}
	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	tracks := make([]*relayv1.Track, len(rows))
	for i, row := range rows {
		tracks[i] = row.toProto()
	}
	return tracks, nil
}

func (m *model) GetTrack(ctx context.Context, id string) (*relayv1.Track, error) {
	row := &trackRow{}
	if err := m.db.Client().NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("track %q: %w", id, err)
	}
	return row.toProto(), nil
}

func (m *model) AddTrack(ctx context.Context, meta *relayv1.AddTrackRequest) (*relayv1.Track, error) {
	track := &relayv1.Track{
		Id:          uuid.New().String(),
		Title:       meta.GetTitle(),
		Artist:      meta.GetArtist(),
		Album:       meta.GetAlbum(),
		StorageNode: m.storageNode,
		AddedAt:     timestamppb.Now(),
	}
	row := trackRowFromProto(track)
	if _, err := m.db.Client().NewInsert().Model(row).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to insert track: %w", err)
	}
	return track, nil
}

func (m *model) SaveUploadedTrack(ctx context.Context, tmpPath string, meta *relayv1.AddTrackRequest, recordingID string) (*relayv1.Track, error) {
	f, err := os.Open(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to reopen uploaded file: %w", err)
	}
	info, err := parseWAV(f)
	f.Close()
	if err != nil {
		// Phase 3 only accepts WAV uploads (see the implementation plan's Phase 3 scope note) --
		// an MP3/FLAC/AAC upload lands here as a parse failure, not silently mis-handled.
		return nil, fmt.Errorf("failed to parse uploaded file as WAV (Phase 3 accepts WAV uploads only): %w", err)
	}
	stat, err := os.Stat(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat uploaded file: %w", err)
	}

	id := uuid.New().String()
	finalPath := filepath.Join(m.storageDir, id+".wav")
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, fmt.Errorf("failed to move uploaded file into the library: %w", err)
	}

	track := &relayv1.Track{
		Id:            id,
		StorageNode:   m.storageNode,
		StoragePath:   finalPath,
		Format:        info.format(),
		DurationMs:    info.durationMs(),
		SizeBytes:     stat.Size(),
		AddedAt:       timestamppb.Now(),
		RecordingId:   recordingID,
		SampleRateHz:  int32(info.SampleRate),
		Channels:      int32(info.Channels),
		BitsPerSample: int32(info.BitsPerSample),
	}
	if meta != nil {
		track.Title = meta.GetTitle()
		track.Artist = meta.GetArtist()
		track.Album = meta.GetAlbum()
	}

	row := trackRowFromProto(track)
	if _, err := m.db.Client().NewInsert().Model(row).Exec(ctx); err != nil {
		// Don't leave an orphaned file on disk if the row never made it into the library.
		os.Remove(finalPath)
		return nil, fmt.Errorf("failed to insert track: %w", err)
	}
	return track, nil
}

func (m *model) DeleteTrack(ctx context.Context, id string) error {
	row := &trackRow{}
	if err := m.db.Client().NewSelect().Model(row).Where("id = ?", id).Scan(ctx); err != nil {
		return fmt.Errorf("track %q: %w", id, err)
	}
	if _, err := m.db.Client().NewDelete().Model((*trackRow)(nil)).Where("id = ?", id).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete track %q: %w", id, err)
	}
	// Best-effort: the row is already gone either way, and a file missing on disk (eg. already
	// cleaned up by hand, or storage_node names a different instance in a future multi-instance
	// deployment -- Phase 9's own disclosed remainder) isn't a reason to report this call failed.
	if row.StoragePath != "" {
		if err := os.Remove(row.StoragePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("deleted track %q but failed to remove its file %q: %w", id, row.StoragePath, err)
		}
	}
	return nil
}
