package draft_runtime

import (
	"database/sql"
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

// REPO - Mechanismes used to persists any type of data. From S3 files storage, NoSQL databases, and SQL databases.

// RepoPluginRegistrar
type RepoPluginRegistrar interface {
	// GetRepoType - is used to determin how to integrate the default rpc handler, and the orm
	GetRepoType() RepoType

	// RegisterDB - gives the plugin the option to use many differnt types of orms/db client. A type assertion can
	// be used at the client level configure the runtime.
	//
	// TODO: Make this parameter an actual type, and not a generic interface
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
	PostgresBun
	Scylla
	Mongo
)

// String - get the human readable value for `RepoType`
func (rt RepoType) String() string {
	return []string{"null", "postgres", "postgres_gorm", "postgres_bun", "scylla", "mongo"}[rt]
}

// WithRepo - Connects to the plugins repo of choice with the runtime
func (c *Commet) withRepo(registrar RepoPluginRegistrar) {
	repoType := registrar.GetRepoType()

	if repoType == NullRepoType {
		return
	} else if repoType == PostgresGorm {
		// set value to local variable
		cfg := c.config.Repos[Postgres.String()].Postgres

		if cfg.SSL {
			panic("ssl configuration for postgres is not implemented")
		}

		addr := fmt.Sprintf("%s://%s@%s:%d/%s?sslmode=disable", cfg.Protocol, cfg.User, cfg.Domain, cfg.Port, cfg.Server)

		db, err := gorm.Open("postgres", addr)
		if err != nil {
			panic("failed to connect to posgres")
		}

		c.gorm = db

		if err := registrar.RegisterDB(db); err != nil {
			panic(err)
		}

	} else if repoType == PostgresBun {
		if cfg.SSL {
			panic("ssl configuration for postgres is not implemented")
		}

		addr := fmt.Sprintf("%s://%s@%s:%d/%s?sslmode=disable", cfg.Protocol, cfg.User, cfg.Domain, cfg.Port, cfg.Server)

		sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(addr)))

		db := bun.NewDB(sqldb, pgdialect.New())

		c.bun = db

	} else {
		panic("a valid repo was not configured")
	}
}
