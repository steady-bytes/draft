package query

import (
	"context"
	"sync"

	"github.com/steady-bytes/draft/pkg/chassis"
	"github.com/steady-bytes/draft/services/core/beacon/store"
)

type (
	// Controller is Beacon's business logic for QueryLogs/StreamLogs: parse the
	// BeaconQL filter once, compile it to parameterized SQL for the historical
	// half of a query, and — for StreamLogs — keep evaluating the same parsed
	// AST in-memory against newly-ingested rows for the live-tail half.
	Controller interface {
		Publisher
		// QueryLogs parses filter once and compiles it to parameterized SQL. after/
		// before independently bound either end of the timestamp range (either may
		// be empty); ascending controls sort order — see store.Storer.QueryLogs's
		// doc comment for the full contract.
		QueryLogs(ctx context.Context, filter string, limit int32, after, before string, ascending bool) ([]store.LogRow, error)
		// StreamLogs replays historical rows matching filter (ascending, so the
		// client sees oldest-to-newest continuity), then blocks streaming live
		// rows matching the same filter via send until ctx is cancelled or send
		// returns an error.
		StreamLogs(ctx context.Context, filter string, limit int32, after string, send func(store.LogRow) error) error
	}

	// Publisher is the narrow interface the ingest package depends on to notify
	// live StreamLogs subscribers of newly-ingested rows, without pulling in the
	// rest of Controller (avoids ingest depending on query's full RPC surface).
	Publisher interface {
		Publish(row store.LogRow)
	}

	controller struct {
		logger    chassis.Logger
		store     store.Storer
		observers *observerRegistry
	}

	// observerRegistry fans a newly-ingested row out to every open StreamLogs
	// subscriber — mirrors services/core/catalyst/broker/controller.go's
	// observerRegistry for QueryStream.
	observerRegistry struct {
		mu      sync.RWMutex
		subs    map[uint64]chan store.LogRow
		counter uint64
	}
)

func NewController(logger chassis.Logger, storer store.Storer) Controller {
	return &controller{
		logger:    logger,
		store:     storer,
		observers: &observerRegistry{subs: make(map[uint64]chan store.LogRow)},
	}
}

func (r *observerRegistry) subscribe() (uint64, chan store.LogRow) {
	ch := make(chan store.LogRow, 256)
	r.mu.Lock()
	id := r.counter
	r.counter++
	r.subs[id] = ch
	r.mu.Unlock()
	return id, ch
}

func (r *observerRegistry) unsubscribe(id uint64) {
	r.mu.Lock()
	delete(r.subs, id)
	r.mu.Unlock()
}

// broadcast sends row to every active StreamLogs subscriber. A slow consumer is
// skipped (buffered channel full) rather than blocking the ingest path.
func (r *observerRegistry) broadcast(row store.LogRow) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ch := range r.subs {
		select {
		case ch <- row:
		default:
		}
	}
}

func (c *controller) Publish(row store.LogRow) {
	c.observers.broadcast(row)
}

func (c *controller) QueryLogs(ctx context.Context, filter string, limit int32, after, before string, ascending bool) ([]store.LogRow, error) {
	ast, err := ParseBeaconQL(filter)
	if err != nil {
		return nil, err
	}
	whereSQL, args, err := Compile(ast)
	if err != nil {
		return nil, err
	}
	return c.store.QueryLogs(ctx, whereSQL, args, limit, after, before, ascending)
}

func (c *controller) StreamLogs(ctx context.Context, filter string, limit int32, after string, send func(store.LogRow) error) error {
	ast, err := ParseBeaconQL(filter)
	if err != nil {
		return err
	}
	whereSQL, args, err := Compile(ast)
	if err != nil {
		return err
	}

	historical, err := c.store.QueryLogs(ctx, whereSQL, args, limit, after, "", true)
	if err != nil {
		return err
	}
	for _, row := range historical {
		if err := send(row); err != nil {
			return err
		}
	}

	id, ch := c.observers.subscribe()
	defer c.observers.unsubscribe(id)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case row := <-ch:
			if Matches(ast, row) {
				if err := send(row); err != nil {
					return err
				}
			}
		}
	}
}
