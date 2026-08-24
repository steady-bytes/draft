package main

import (
	"context"
	"sync"

	"github.com/steady-bytes/draft/pkg/chassis"
)

// memoryRepository is a trivial chassis.Repository implementation that exists only
// to exercise the WithRepository -> chassis.Effect wiring end to end. It stores
// nothing of real value; Open and Close log so the effect stack's LIFO teardown is
// observable in this service's output alongside the KV heartbeat effect in main.go.
type memoryRepository struct {
	logger chassis.Logger
	mu     sync.Mutex
	data   map[string]string
}

func newMemoryRepository(logger chassis.Logger) *memoryRepository {
	return &memoryRepository{
		logger: logger.WithFields(chassis.Fields{"repository": "memory"}),
		data:   make(map[string]string),
	}
}

func (r *memoryRepository) Open(ctx context.Context, config chassis.Config) error {
	r.logger.Info("memory repository opened")
	return nil
}

func (r *memoryRepository) Ping(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return nil
}

func (r *memoryRepository) Close(ctx context.Context) error {
	r.logger.Info("memory repository closed")
	return nil
}
