package chassis

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/dgraph-io/badger/v2"
	"github.com/jinzhu/gorm"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// REPO - Mechanism used to persists any type of data. From S3 files storage, NoSQL databases, and SQL databases.

// RepoRegistrar - The interface to implement if the service needs persistent data storage
type (
	// RepoType - selects the type of persistent's layer the service will need
	RepoKind int

	RepoRegistrar interface {
		// RegisterRepo - gives the plugin the option to use many different types of orms/db client. A type assertion can
		// be used at the client level configure the runtime.
		// If
		RegisterRepo(interface{}) error
	}
)

// Options for repositories that can be used in a service.
// Since services will need to have many different kinds of persistence
// options. To control what clients can be supported seemed ideal to
// add a builder for the supported kinds.
//
// Long term the idea of the platform is to detect what kind of persistence
// layer is needed from the infrastructure and it
//
// NOTE: When adding, or removing a value in `RepoType` make sure
//
//	      to update the corresponding `String()` method. If they are out
//		  of sync then a potential "out of bounds" error will occur.
const (
	NullRepoType RepoKind = iota
	// FULLY SUPPORTED
	Badger
	// NOT SUPPORTED BUT MIGHT
	PostgresSQLX
	// FULLY SUPPORTED BUT DEPRECATING
	PostgresGORM
	// FULLY SUPPORTED
	PostgresBUN
	// NOT SUPPORTED BUT MIGHT
	Scylla
	// NOT SUPPORTED BUT MIGHT
	Mongo
)

var (
	ErrIncorrectDBInterface = errors.New("incorrect db interface")
	ErrDBNilDBConnection    = errors.New("db connection is nil")
)

// String - get the human readable value for `RepoType`
func (rt RepoKind) String() string {
	return []string{"null", "badger", "postgres_sqlx", "postgres_gorm", "postgres_bun", "scylla", "mongo"}[rt]
}

// WithRepo - Connects to the plugins repo of choice with the runtime
func (c *Runtime) withRepo(kind RepoKind, registrar RepoRegistrar) {
	c.repoKind = kind

	switch c.repoKind {
	case NullRepoType:
		return
	case Badger:
		c.bootstrapBadger(registrar)
	case PostgresGORM:
		c.bootstrapPostgresGorm(registrar)
	case PostgresBUN:
		c.bootstrapPostgresBun(registrar)
	default:
		panic("a valid repo was not configured")
	}
}

// bootstrapBadger - Open up a connection with the default options for `Badger`
// This will setup things like file system bindings so badger can write data
// to the file system.
func (c *Runtime) bootstrapBadger(registrar RepoRegistrar) {
	nodeID := os.Getenv(nodeIDEnv)
	if nodeID == "" {
		panic("raft node id not set")
	}

	badgerOpt := badger.DefaultOptions(nodeID)
	db, err := badger.Open(badgerOpt)
	if err != nil {
		panic(err)
	}
	// store a reference of badger into the runtime
	c.badger = db

	// Call `RegisterRepo` function that should be implemented by
	// the consuming service
	if err := registrar.RegisterRepo(db); err != nil {
		panic(err)
	}
}

// bootstrapPostgresGorm - A utility for registering `GORM` with the `draft` runtime.
func (c *Runtime) bootstrapPostgresGorm(registrar RepoRegistrar) {
	// set value to local variable
	cfg := c.config.Repos[PostgresGORM.String()].Postgres

	if cfg.SSL {
		panic("ssl configuration for postgres is not implemented")
	}

	addr := fmt.Sprintf("%s://%s@%s:%d/%s?sslmode=disable", cfg.Protocol, cfg.User, cfg.Domain, cfg.Port, cfg.Server)

	db, err := gorm.Open("postgres", addr)
	if err != nil {
		panic("failed to connect to posgres")
	}

	c.gorm = db

	if err := registrar.RegisterRepo(db); err != nil {
		panic(err)
	}
}

// bootstrapPostgresBun - A utility for registering `bun` orm with the `draft` runtime.
func (c *Runtime) bootstrapPostgresBun(registrar RepoRegistrar) {
	cfg := c.config.Repos[PostgresBUN.String()].Postgres

	if cfg.SSL {
		panic("ssl configuration for postgres is not implemented")
	}

	addr := fmt.Sprintf("%s://%s@%s:%d/%s?sslmode=disable", cfg.Protocol, cfg.User, cfg.Domain, cfg.Port, cfg.Server)
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(addr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	c.bun = db

	if err := registrar.RegisterRepo(db); err != nil {
		panic(err)
	}
}
