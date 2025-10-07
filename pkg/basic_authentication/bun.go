package basic_authentication

import (
	"github.com/steady-bytes/draft/pkg/chassis"
)

type BunBasicAuthenticationRepository struct {
	// Add any fields needed for the repository, such as a database connection
	logger chassis.Logger
}

func NewBunBasicAuthenticationRepository(logger chassis.Logger) *BunBasicAuthenticationRepository {
	return &BunBasicAuthenticationRepository{
		logger: logger,
	}
}
