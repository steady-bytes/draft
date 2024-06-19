package chassis

import "context"


type SecretStore interface {
	// Open will initialize the client with the given configuration
	Open(ctx context.Context, config Config) (err error)
	// Get retrieves a secret from the vault for the given key
	Get(ctx context.Context, key string) (value string, err error)
}
