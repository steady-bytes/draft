package draft_runtime_golang

import (
	"database/sql"
	"fmt"

	"github.com/dgraph-io/badger"
	"github.com/jinzhu/gorm"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// REPO - Mechanism used to persists any type of data. From S3 files storage, NoSQL databases, and SQL databases.

// RepoRegistrar -
type RepoRegistrar interface {
	// RegisterRepo - gives the plugin the option to use many different types of orms/db client. A type assertion can
	// be used at the client level configure the runtime.
	RegisterRepo(interface{}) error
}

// RepoType - selects the type of persistent's layer the service will need
type RepoKind int

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
	Badger
	Postgres
	PostgresGorm
	PostgresBun
	Scylla
	Mongo
)

// String - get the human readable value for `RepoType`
func (rt RepoKind) String() string {
	return []string{"null", "badger", "postgres", "postgres_gorm", "postgres_bun", "scylla", "mongo"}[rt]
}

// WithRepo - Connects to the plugins repo of choice with the runtime
// TODO: Change this method body to be a switch statement that will call specific bootstrapping
// methods for each type of repo instead of keeping itall in
func (c *Runtime) withRepo(kind RepoKind, registrar RepoRegistrar) {
	c.repoKind = kind

	switch c.repoKind {
	case NullRepoType:
		return
	case Badger:
		c.bootstrapBadger(registrar)
	case PostgresGorm:
		c.bootstrapPostgresGorm(registrar)
	case PostgresBun:
		c.bootstrapPostgresBun(registrar)
	default:
		panic("a valid repo was not configured")
	}
}

func (c *Runtime) bootstrapBadger(registrar RepoRegistrar) {
	badgerOpt := badger.DefaultOptions(c.Title())
	db, err := badger.Open(badgerOpt)
	if err != nil {
		panic(err)
	}

	c.badger = db

	if err := registrar.RegisterRepo(db); err != nil {
		panic(err)
	}
}

// bootstrapPostgresGorm - A utility for registering `GORM` with the `draft` runtime.
// This method does not return anything but can panic because it's considered a fatal issue
// if the db can't be configured and setup correctly in the runtime.
func (c *Runtime) bootstrapPostgresGorm(registrar RepoRegistrar) {
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

	if err := registrar.RegisterRepo(db); err != nil {
		panic(err)
	}
}

// bootstrapPostgresBun - A utility for registering `bun` orm with the `draft` runtime.
func (c *Runtime) bootstrapPostgresBun(registrar RepoRegistrar) {
	cfg := c.config.Repos[Postgres.String()].Postgres

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
