package basic_authentication

import (
	"context"
	"errors"

	bav1 "github.com/steady-bytes/draft/api/core/authentication/basic/v1"
)

var (
	ErrInvalidLookupKey = errors.New("invalid lookup key")
	ErrUserNotFound     = errors.New("user not found")
	ErrInternalServer   = errors.New("internal server error")
)

// The BasicAuthenticationRepository interface defines the methods that are required for the
// any implementation of a basic authentication repository
type BasicAuthenticationRepository interface {
	// get is a simple lookup method that retrieves an entity based on a key-value pair.
	// valid keys could be "id", "username", or "email".
	Get(ctx context.Context, key bav1.LookupEntityKeys, val string) (*bav1.Entity, error)

	SaveEntity(ctx context.Context, entity *bav1.Entity) (*bav1.Entity, error)
	SaveSession(ctx context.Context, session *bav1.Session, username string) (*bav1.Session, error)
	DeleteSession(ctx context.Context, session *bav1.Session, username string) error
}
