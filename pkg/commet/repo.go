package commet

import (
	"fmt"

	"github.com/jinzhu/gorm"
)

// REPO - Mechanismes used to persists any type of data. From S3 files storage, NoSQL databases, and SQL databases.

// RepoPluginRegistrar
type RepoPluginRegistrar interface {
	// GetRepoType - is used to determin how to integrate the default rpc handler, and the orm
	GetRepoType() RepoType

	// GetModel - receives the interface that will be used as the model for the repo, model is provided by the concrete implementation
	GetModel() interface{}

	// RegisterDB - gives the plugin the option to use many differnt types of orms/db client. A type assertion can
	// be used at the client level configure the runtime.
	RegisterDB(interface{}) error
}

// RepoType - selects the type of persistant's layer the service will need
type RepoType int

// Options for repositories that can be used in a service
// NOTE: When adding, or removing a value in `RepoType` make sure
//       to update the corresponding `String()` method. If they are out
//			 of sync then a potential "out of bounds" error will occure.
const (
	NullRepoType RepoType = iota
	Postgres
	PostgresGorm
	Scylla
	Mongo
)

// String - get the human readable value for `RepoType`
func (rt RepoType) String() string {
	return []string{"null", "postgres", "postgres_gorm", "scylla", "mongo"}[rt]
}

// WithRepo - Connects to the plugins repo of choice with the runtime
func (c *Commet) withRepo() {
	repoType := c.defaultPlugin.GetRepoType()

	if repoType == NullRepoType {
		return
	} else if repoType == PostgresGorm {
		// set value to local variable
		cfg := c.config.Repos[Postgres.String()].Postgres

		if cfg.SSL {
			panic("ssl configuration for postgres is not implemented")
		}

		addr := fmt.Sprintf("%s://%s@%s:%d/%s?sslmode=disable", cfg.Protocol, cfg.User, cfg.Domain, cfg.Port, cfg.Server)
		fmt.Println("addr: ", addr)
		db, err := gorm.Open("postgres", addr)
		if err != nil {
			panic("failed to connect to posgres")
		}

		c.gorm = db

		if cfg.Migrate {
			if err := c.gorm.AutoMigrate(c.defaultPlugin.GetModel()); err != nil {
				fmt.Println(err)
			}
		}
	} else {
		panic("a valid repo was not configured")
	}
}
