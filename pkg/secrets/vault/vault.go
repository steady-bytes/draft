package vault

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

type (
	SecretStore interface {
		chassis.SecretStore
		Set(ctx context.Context, key string, value string) error
		Delete(ctx context.Context, key string) error
	}
	secretStore struct {
		client    *vault.Client
		configKey string
	}
)

// New instantiates a new secret store. A call to Open is required before use.
// The configKey parameter dictates which key in the configuration will be read during
// initialization. Default: "secrets.vault"
func New(configKey string) SecretStore {
	if configKey == "" {
		configKey = "secrets.vault"
	}
	return &secretStore{
		configKey: configKey,
	}
}

func (s *secretStore) Open(ctx context.Context, config chassis.Config) error {
	// prepare a client with the given base address
	url := config.GetString(fmt.Sprintf("%s.url", s.configKey))
	client, err := vault.New(
		vault.WithAddress(url),
		vault.WithRequestTimeout(5*time.Second),
	)
	if err != nil {
		return err
	}

	// TODO: use app role login instead of root token
	/*
		resp, err := client.Auth.AppRoleLogin(
			ctx,
			schema.AppRoleLoginRequest{
				RoleId:   os.Getenv("MY_APPROLE_ROLE_ID"),
				SecretId: os.Getenv("MY_APPROLE_SECRET_ID"),
			},
			vault.WithMountPath("my/approle/path"), // optional, defaults to "approle"
		)
	*/
	// authenticate with a root token (insecure)
	if err := client.SetToken("myroot"); err != nil {
		return err
	}
	s.client = client

	return nil
}

func (c *secretStore) Get(ctx context.Context, key string) (string, error) {
	path, attribute := splitKey(key)
	s, err := c.client.Secrets.KvV2Read(ctx, path, vault.WithMountPath("secret"))
	if err != nil {
		return "", err
	}
	value, ok := s.Data.Data[attribute].(string)
	if !ok {
		return "", fmt.Errorf("unable to read secret")
	}
	return value, nil
}

func (c *secretStore) Set(ctx context.Context, key string, value string) error {
	path, attribute := splitKey(key)
	_, err := c.client.Secrets.KvV2Write(ctx, path, schema.KvV2WriteRequest{
		Data: map[string]any{
			attribute: value,
		}},
		vault.WithMountPath("secret"),
	)
	if err != nil {
		return err
	}
	return nil
}

func (c *secretStore) Delete(ctx context.Context, key string) error {
	_, err := c.client.Secrets.KvV2Delete(ctx, key, vault.WithMountPath("secret"))
	if err != nil {
		return err
	}
	return nil
}

// HELPERS

func splitKey(key string) (path string, attribute string) {
	segments := strings.Split(key, "/")
	if len(segments) == 0 {
		return "", ""
	}
	// path is everything but the last segment
	path = strings.Join(segments[:len(segments)-1], "/")
	// attribute is the last segment
	attribute = segments[len(segments)-1]
	return path, attribute
}
