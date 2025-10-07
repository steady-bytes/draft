package basic_authentication

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	bav1 "github.com/steady-bytes/draft/api/core/authentication/basic/v1"
	bpc "github.com/steady-bytes/draft/pkg/bpc"
	"github.com/steady-bytes/draft/pkg/chassis"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
)

type BlueprintBasicAuthenticationRepository struct {
	// Add any fields needed for the repository, such as a database connection
	logger chassis.Logger
	client kvv1Connect.KeyValueServiceClient
}

func NewBlueprintBasicAuthenticationRepository(logger chassis.Logger, client kvv1Connect.KeyValueServiceClient) *BlueprintBasicAuthenticationRepository {
	return &BlueprintBasicAuthenticationRepository{
		logger: logger,
		client: client,
	}
}

func (r *BlueprintBasicAuthenticationRepository) Get(ctx context.Context, key bav1.LookupEntityKeys, val string) (*bav1.Entity, error) {
	if key == bav1.LookupEntityKeys_LOOKUP_ENTITY_KEY_UNSPECIFIED {
		return nil, ErrInvalidLookupKey
	}

	if key == bav1.LookupEntityKeys_LOOKUP_ENTITY_KEY_USERNAME {
		// check if the user already exists
		entity, err := bpc.GetById[*bav1.Entity](
			ctx,
			r.client,
			val,
			func() *bav1.Entity {
				return &bav1.Entity{}
			})
		if err != nil {
			if cerr, ok := err.(*connect.Error); ok && cerr.Code() == connect.CodeNotFound {
				r.logger.Debug("user not found")
				return nil, ErrUserNotFound
			}
			r.logger.
				WithError(err).
				Error("internal server error")

			return nil, ErrInternalServer
		}
		return entity, nil
	}

	var k string
	if key == bav1.LookupEntityKeys_LOOKUP_ENTITY_KEY_EMAIL {
		k = "email"
	} else if key == bav1.LookupEntityKeys_LOOKUP_ENTITY_KEY_ID {
		k = "id"
	} else {
		return nil, fmt.Errorf("unsupported lookup key: %s", key.String())
	}

	user, err := bpc.ListAndFilter[*bav1.Entity](
		ctx,
		r.client,
		&kvv1.Statement{
			Where: &kvv1.Statement_KeyVal{
				KeyVal: &kvv1.Equal{
					Match: map[string]string{
						k: val,
					},
				},
			},
		}, func() *bav1.Entity {
			return &bav1.Entity{}
		})
	if err != nil {
		return nil, err
	}
	if len(user) == 0 {
		return nil, errors.New("user not found")
	}

	return user[0], nil
}

func (r *BlueprintBasicAuthenticationRepository) SaveEntity(ctx context.Context, entity *bav1.Entity) (*bav1.Entity, error) {
	_, err := bpc.Save(ctx, r.client, entity.Username, entity)
	if err != nil {
		return nil, err
	}

	return entity, nil
}

func (r *BlueprintBasicAuthenticationRepository) SaveSession(ctx context.Context, session *bav1.Session, username string) (*bav1.Session, error) {
	key := username + "-session-" + session.GetId()
	_, err := bpc.Save(ctx, r.client, key, session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (r *BlueprintBasicAuthenticationRepository) DeleteSession(ctx context.Context, session *bav1.Session, username string) error {
	key := username + "-session-" + session.GetId()

	if _, err := r.client.Delete(ctx, connect.NewRequest(&kvv1.DeleteRequest{
		Key: key,
	})); err != nil {
		r.logger.
			WithError(err).
			Error("failed to delete session: " + key)
		return err
	}

	r.logger.Debug("session deleted successfully: " + key)
	return nil
}
