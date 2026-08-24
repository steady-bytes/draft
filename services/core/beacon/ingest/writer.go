// Package ingest implements Beacon's OTLP log receiver and the batch writer
// that persists accepted rows into ClickHouse. The writer's shape is
// deliberately structurally identical to
// services/core/catalyst/broker/store.go's flusher/insertBatch: a bounded
// channel, a ticker-driven batch flush, and a non-blocking Save that drops
// under sustained overload rather than back-pressuring the OTLP receiver.
package ingest

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
)

const (
	// batchSize is the maximum number of rows accumulated before a flush is
	// forced, regardless of the flush interval.
	batchSize = 5_000

	// flushInterval is the maximum time a row may sit in the buffer before it
	// is written to ClickHouse, even if the batch is not yet full.
	flushInterval = 500 * time.Millisecond

	// chanCapacity is the depth of the ingest channel, sized well above the
	// largest expected export burst so Save never blocks the OTLP receiver.
	chanCapacity = 100_000
)

// Writer batches store.LogRow values and flushes them into ClickHouse.
type Writer struct {
	store        store.Storer
	logger       chassis.Logger
	ch           chan store.LogRow
	droppedTotal atomic.Uint64
}

func NewWriter(s store.Storer, logger chassis.Logger) *Writer {
	w := &Writer{
		store:  s,
		logger: logger,
		ch:     make(chan store.LogRow, chanCapacity),
	}
	go w.flusher()
	return w
}

// Save enqueues row for batch insertion. It never blocks the caller — if the
// channel is full the row is dropped rather than stalling the OTLP receiver.
func (w *Writer) Save(row store.LogRow) {
	select {
	case w.ch <- row:
	default:
		w.droppedTotal.Add(1)
	}
}

// DroppedTotal returns the number of rows dropped so far because the ingest
// channel was full.
func (w *Writer) DroppedTotal() uint64 {
	return w.droppedTotal.Load()
}

func (w *Writer) flusher() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]store.LogRow, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := w.store.InsertLogs(context.Background(), buf); err != nil {
			w.logger.WithField("error", err.Error()).Error("failed to flush log batch to clickhouse")
		}
		buf = buf[:0]
	}

	for {
		select {
		case row := <-w.ch:
			buf = append(buf, row)
			if len(buf) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
