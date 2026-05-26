package broker

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// batchSize is the maximum number of events accumulated before a flush is
	// forced, regardless of the flush interval.
	batchSize = 5_000

	// flushInterval is the maximum time an event may sit in the buffer before
	// it is written to ClickHouse, even if the batch is not yet full.
	flushInterval = 500 * time.Millisecond

	// chanCapacity is the depth of the ingest channel. It is sized well above
	// the largest expected burst so Save never blocks the produce goroutine.
	chanCapacity = 100_000
)

type (
	ClickHouseConfig struct {
		Enabled  bool   `mapstructure:"enabled"`
		Address  string `mapstructure:"address"`
		Database string `mapstructure:"database"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}

	// EdgeVolume is a (source, event_type, count) tuple returned by QueryVolumes.
	EdgeVolume struct {
		Source    string
		EventType string
		Count     uint32
	}

	// LatencyStats holds pre-computed median and P95 dispatch latency in milliseconds.
	LatencyStats struct {
		MedianMs float64
		P95Ms    float64
	}

	// TopicSeriesPoint is one time-bucketed count for QueryTopicSeries.
	TopicSeriesPoint struct {
		TimestampMs int64
		Count       uint32
	}

	// StoreResourceStats contains point-in-time resource metrics read directly
	// from the store's in-process state rather than from ClickHouse.
	StoreResourceStats struct {
		DroppedTotal  uint64
		QueueDepth    uint32
		QueueCapacity uint32
		FlushP95Ms    float64
		FlushCount    uint32
	}

	Storer interface {
		Save(ctx context.Context, event *acv1.CloudEvent, receivedAt, forwardedAt time.Time) error
		Query(ctx context.Context, limit int32, after string, descending bool) ([]*acv1.CloudEvent, error)
		// QueryDistinctSources returns the list of distinct producer source names and
		// a map of event_type → []source_names for edge derivation in GetTopology.
		QueryDistinctSources(ctx context.Context) ([]string, map[string][]string, error)
		// QueryVolumes returns per-(source, event_type) event counts over the last windowSecs seconds.
		QueryVolumes(ctx context.Context, windowSecs int32) ([]EdgeVolume, error)
		// QueryLatency returns median and P95 dispatch latency over the last windowSecs seconds.
		QueryLatency(ctx context.Context, windowSecs int32) (LatencyStats, error)
		// StoreStats returns live resource counters from the store's in-process state.
		StoreStats() StoreResourceStats
		// QueryTopicSeries returns time-bucketed event counts for one event_type.
		// Pass windowSecs = -1 to query all stored data. Returns the points and
		// the bucket size in seconds that was chosen.
		QueryTopicSeries(ctx context.Context, eventType string, windowSecs int32) ([]TopicSeriesPoint, int32, error)
	}

	// flushRing is a fixed-size ring buffer that tracks store flush durations for
	// P95 computation without requiring an external dependency.
	flushRing struct {
		mu    sync.Mutex
		ring  [128]float64
		pos   int
		total atomic.Uint64
	}

	noopStore struct{}

	// pendingEvent captures both the time the event arrived at Catalyst (receivedAt)
	// and the time it was dispatched to consumers (forwardedAt). Both come from the
	// same Go clock so latency (forwardedAt - receivedAt) is always non-negative,
	// unlike ClickHouse's now64() which fires at batch-flush time after forwardedAt.
	pendingEvent struct {
		event       *acv1.CloudEvent
		receivedAt  time.Time
		forwardedAt time.Time
	}

	clickhouseStore struct {
		conn         driver.Conn
		ch           chan pendingEvent
		droppedTotal atomic.Uint64
		flushes      flushRing
	}
)

func (r *flushRing) record(ms float64) {
	r.mu.Lock()
	r.ring[r.pos%128] = ms
	r.pos++
	r.mu.Unlock()
	r.total.Add(1)
}

func (r *flushRing) p95Ms() float64 {
	r.mu.Lock()
	n := r.pos
	if n > 128 {
		n = 128
	}
	buf := make([]float64, n)
	copy(buf, r.ring[:n])
	r.mu.Unlock()
	if n == 0 {
		return 0
	}
	sort.Float64s(buf)
	return buf[int(float64(n-1)*0.95)]
}

func NewNoopStore() Storer { return &noopStore{} }

func (n *noopStore) Save(_ context.Context, _ *acv1.CloudEvent, _, _ time.Time) error { return nil }

func (n *noopStore) Query(_ context.Context, _ int32, _ string, _ bool) ([]*acv1.CloudEvent, error) {
	return nil, nil
}

func (n *noopStore) QueryDistinctSources(_ context.Context) ([]string, map[string][]string, error) {
	return nil, nil, nil
}

func (n *noopStore) QueryVolumes(_ context.Context, _ int32) ([]EdgeVolume, error) {
	return nil, nil
}

func (n *noopStore) QueryLatency(_ context.Context, _ int32) (LatencyStats, error) {
	return LatencyStats{}, nil
}

func (n *noopStore) StoreStats() StoreResourceStats { return StoreResourceStats{} }

func (n *noopStore) QueryTopicSeries(_ context.Context, _ string, _ int32) ([]TopicSeriesPoint, int32, error) {
	return nil, 60, nil
}

func NewClickHouseStore(cfg ClickHouseConfig) (Storer, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Address},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	store := &clickhouseStore{
		conn: conn,
		ch:   make(chan pendingEvent, chanCapacity),
	}
	if err := store.migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	go store.flusher()
	return store, nil
}

const createEventsTable = `
CREATE TABLE IF NOT EXISTS events (
    id           String,
    source       String,
    spec_version String,
    type         String,
    body         String,
    inserted_at  DateTime64(9) DEFAULT now64(),
    forwarded_at DateTime64(9) DEFAULT toDateTime64(0, 9),
    raw          String
) ENGINE = MergeTree()
ORDER BY (type, inserted_at)
PARTITION BY toYYYYMM(inserted_at)
`

func (s *clickhouseStore) migrate(ctx context.Context) error {
	if err := s.conn.Exec(ctx, createEventsTable); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, `ALTER TABLE events ADD COLUMN IF NOT EXISTS body String DEFAULT ''`); err != nil {
		return err
	}
	return s.conn.Exec(ctx, `ALTER TABLE events ADD COLUMN IF NOT EXISTS forwarded_at DateTime64(9) DEFAULT toDateTime64(0, 9)`)
}

// Save enqueues the event for batch insertion. It never blocks the caller — if
// the channel is full the event is dropped rather than stalling the produce loop.
func (s *clickhouseStore) Save(_ context.Context, event *acv1.CloudEvent, receivedAt, forwardedAt time.Time) error {
	select {
	case s.ch <- pendingEvent{event: event, receivedAt: receivedAt, forwardedAt: forwardedAt}:
	default:
		s.droppedTotal.Add(1)
	}
	return nil
}

func (s *clickhouseStore) StoreStats() StoreResourceStats {
	return StoreResourceStats{
		DroppedTotal:  s.droppedTotal.Load(),
		QueueDepth:    uint32(len(s.ch)),
		QueueCapacity: chanCapacity,
		FlushP95Ms:    s.flushes.p95Ms(),
		FlushCount:    uint32(s.flushes.total.Load()),
	}
}

// flusher drains the ingest channel and writes events to ClickHouse in batches.
// It flushes whenever the buffer reaches batchSize or flushInterval elapses.
func (s *clickhouseStore) flusher() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]pendingEvent, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		start := time.Now()
		if err := s.insertBatch(context.Background(), buf); err != nil {
			// Non-fatal: log and continue. Individual event loss is acceptable
			// over stalling the pipeline.
			_ = err
		}
		s.flushes.record(float64(time.Since(start).Milliseconds()))
		buf = buf[:0]
	}

	for {
		select {
		case pe := <-s.ch:
			buf = append(buf, pe)
			if len(buf) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (s *clickhouseStore) insertBatch(ctx context.Context, events []pendingEvent) error {
	batch, err := s.conn.PrepareBatch(ctx,
		"INSERT INTO events (id, source, spec_version, type, body, inserted_at, forwarded_at, raw)",
	)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	for _, pe := range events {
		b, err := proto.Marshal(pe.event)
		if err != nil {
			continue
		}
		if err := batch.Append(
			pe.event.GetId(),
			pe.event.GetSource(),
			pe.event.GetSpecVersion(),
			pe.event.GetType(),
			pe.event.GetTextData(),
			pe.receivedAt,
			pe.forwardedAt,
			base64.StdEncoding.EncodeToString(b),
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}
	return batch.Send()
}

func (s *clickhouseStore) Query(ctx context.Context, limit int32, after string, descending bool) ([]*acv1.CloudEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	order := "ASC"
	if descending {
		order = "DESC"
	}

	var (
		rows driver.Rows
		err  error
	)
	if after != "" {
		t, parseErr := time.Parse(time.RFC3339, after)
		if parseErr != nil {
			return nil, fmt.Errorf("parse after timestamp: %w", parseErr)
		}
		rows, err = s.conn.Query(ctx,
			"SELECT raw, forwarded_at FROM events WHERE inserted_at > ? ORDER BY inserted_at "+order+" LIMIT ?",
			t, limit,
		)
	} else {
		rows, err = s.conn.Query(ctx,
			"SELECT raw, forwarded_at FROM events ORDER BY inserted_at "+order+" LIMIT ?",
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	epochZero := time.Unix(0, 0)

	var result []*acv1.CloudEvent
	for rows.Next() {
		var raw string
		var forwardedAt time.Time
		if err := rows.Scan(&raw, &forwardedAt); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("decode event: %w", err)
		}
		event := &acv1.CloudEvent{}
		if err := proto.Unmarshal(b, event); err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}
		// For rows stored before forwarded_at was injected into the proto,
		// back-fill the attribute from the dedicated ClickHouse column.
		if _, ok := event.Attributes["forwarded_at"]; !ok && forwardedAt.After(epochZero) {
			if event.Attributes == nil {
				event.Attributes = make(map[string]*acv1.CloudEvent_CloudEventAttributeValue)
			}
			event.Attributes["forwarded_at"] = &acv1.CloudEvent_CloudEventAttributeValue{
				Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{
					CeTimestamp: timestamppb.New(forwardedAt),
				},
			}
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

// QueryDistinctSources returns the distinct producer names and a map of
// event_type → []producer_names derived from the events table.
func (s *clickhouseStore) QueryDistinctSources(ctx context.Context) ([]string, map[string][]string, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT DISTINCT source, type FROM events ORDER BY source, type LIMIT 2000",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query distinct sources: %w", err)
	}
	defer rows.Close()

	seenSources := make(map[string]bool)
	sourcesByType := make(map[string][]string)

	for rows.Next() {
		var src, typ string
		if err := rows.Scan(&src, &typ); err != nil {
			return nil, nil, fmt.Errorf("scan distinct sources: %w", err)
		}
		seenSources[src] = true
		already := false
		for _, s := range sourcesByType[typ] {
			if s == src {
				already = true
				break
			}
		}
		if !already {
			sourcesByType[typ] = append(sourcesByType[typ], src)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	producers := make([]string, 0, len(seenSources))
	for src := range seenSources {
		producers = append(producers, src)
	}
	sort.Strings(producers)
	return producers, sourcesByType, nil
}

// QueryVolumes returns event counts grouped by (source, event_type) over the last windowSecs seconds.
// Pass windowSecs = -1 to query all stored events without a time filter.
func (s *clickhouseStore) QueryVolumes(ctx context.Context, windowSecs int32) ([]EdgeVolume, error) {
	var (
		rows driver.Rows
		err  error
	)
	if windowSecs > 0 {
		rows, err = s.conn.Query(ctx,
			"SELECT source, type, toUInt32(count()) AS cnt FROM events WHERE inserted_at >= now() - INTERVAL ? SECOND GROUP BY source, type ORDER BY cnt DESC LIMIT 2000",
			windowSecs,
		)
	} else {
		rows, err = s.conn.Query(ctx,
			"SELECT source, type, toUInt32(count()) AS cnt FROM events GROUP BY source, type ORDER BY cnt DESC LIMIT 2000",
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query volumes: %w", err)
	}
	defer rows.Close()

	var result []EdgeVolume
	for rows.Next() {
		var src, typ string
		var cnt uint32
		if err := rows.Scan(&src, &typ, &cnt); err != nil {
			return nil, fmt.Errorf("scan volume row: %w", err)
		}
		result = append(result, EdgeVolume{Source: src, EventType: typ, Count: cnt})
	}
	return result, rows.Err()
}

// QueryLatency returns median and P95 dispatch latency in milliseconds over the last windowSecs seconds.
// Rows where forwarded_at is epoch zero (not yet stamped) are excluded.
// Pass windowSecs = -1 to query all stored events without a time filter.
// bucketSecs returns the bucket interval for the given window.
func bucketSecs(windowSecs int32) int32 {
	switch {
	case windowSecs <= 0:
		return 60 // all time: 1-min buckets
	case windowSecs <= 900:
		return 1 // 15 min: 1-sec buckets
	case windowSecs <= 3600:
		return 5 // 1 h: 5-sec buckets
	case windowSecs <= 10800:
		return 10 // 3 h: 10-sec buckets
	default:
		return 30 // 24 h: 30-sec buckets
	}
}

// QueryTopicSeries returns time-bucketed event counts for one event_type.
func (s *clickhouseStore) QueryTopicSeries(ctx context.Context, eventType string, windowSecs int32) ([]TopicSeriesPoint, int32, error) {
	bucket := bucketSecs(windowSecs)
	var (
		rows driver.Rows
		err  error
	)
	if windowSecs > 0 {
		rows, err = s.conn.Query(ctx,
			fmt.Sprintf(`SELECT
				toInt64(toUnixTimestamp(toStartOfInterval(inserted_at, toIntervalSecond(%d)))) * 1000 AS ts,
				toUInt32(count()) AS cnt
			FROM events
			WHERE type = ?
			  AND inserted_at >= now() - INTERVAL %d SECOND
			GROUP BY ts ORDER BY ts ASC`, bucket, windowSecs),
			eventType,
		)
	} else {
		rows, err = s.conn.Query(ctx,
			fmt.Sprintf(`SELECT
				toInt64(toUnixTimestamp(toStartOfInterval(inserted_at, toIntervalSecond(%d)))) * 1000 AS ts,
				toUInt32(count()) AS cnt
			FROM events
			WHERE type = ?
			GROUP BY ts ORDER BY ts ASC LIMIT 10080`, bucket),
			eventType,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("query topic series: %w", err)
	}
	defer rows.Close()

	var result []TopicSeriesPoint
	for rows.Next() {
		var ts int64
		var cnt uint32
		if err := rows.Scan(&ts, &cnt); err != nil {
			return nil, 0, fmt.Errorf("scan series row: %w", err)
		}
		result = append(result, TopicSeriesPoint{TimestampMs: ts, Count: cnt})
	}
	return result, bucket, rows.Err()
}

func (s *clickhouseStore) QueryLatency(ctx context.Context, windowSecs int32) (LatencyStats, error) {
	const baseQuery = `SELECT
		quantile(0.5)(toFloat64(toUnixTimestamp64Nano(forwarded_at) - toUnixTimestamp64Nano(inserted_at))) / 1e6,
		quantile(0.95)(toFloat64(toUnixTimestamp64Nano(forwarded_at) - toUnixTimestamp64Nano(inserted_at))) / 1e6
	FROM events
	WHERE forwarded_at > toDateTime64(0, 9)`

	var row driver.Row
	if windowSecs > 0 {
		row = s.conn.QueryRow(ctx, baseQuery+" AND inserted_at >= now() - INTERVAL ? SECOND", windowSecs)
	} else {
		row = s.conn.QueryRow(ctx, baseQuery)
	}

	var stats LatencyStats
	if err := row.Scan(&stats.MedianMs, &stats.P95Ms); err != nil {
		return LatencyStats{}, fmt.Errorf("query latency: %w", err)
	}
	return stats, nil
}
