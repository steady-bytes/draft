package chassis

import (
	"context"
)

type Repository interface {
	Open(ctx context.Context, config Config) (err error)
	Ping(ctx context.Context) (err error)
	Close(ctx context.Context) (err error)
}
