package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/steady-bytes/draft/pkg/chassis"
	pgbun "github.com/steady-bytes/draft/pkg/repositories/postgres/bun"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// This file spins up a real, throwaway Postgres container (via `docker run`)
// for the RPC-level tests in rpc_test.go to run against, rather than mocking
// the database — per the Phase 2 brief, a real backing store is the more
// convincing test of store.go's queries (pagination, the unique-constraint
// detection in particular) than a fake ever would be. The container is
// always torn down, including on a failed/panicking test run, via TestMain's
// defer.

const (
	testContainerName = "garage-plugincatalog-test-postgres"
	testPostgresPort  = "55437"
	testDSN           = "postgres://garage_test:garage_test@localhost:" + testPostgresPort + "/garage_test?sslmode=disable"
)

// testDB is the shared connection every test in this package uses. Tests
// isolate themselves by using unique plugin names rather than by resetting
// the database between tests.
var testDB *bun.DB

// testRepository adapts an already-open *bun.DB into pgbun.Repository, the
// interface store.go and createSchema expect. Its Open is a no-op: the
// connection is already established by TestMain before any test runs, so
// there's no chassis.Config to read a DSN from the way the real
// pgbun.Repository.Open would.
type testRepository struct {
	client *bun.DB
}

func (r *testRepository) Client() *bun.DB { return r.client }

func (r *testRepository) Open(_ context.Context, _ chassis.Config) error { return nil }

func (r *testRepository) Close(ctx context.Context) error { return r.client.Close() }

func (r *testRepository) Ping(ctx context.Context) error { return r.client.PingContext(ctx) }

var _ pgbun.Repository = (*testRepository)(nil)

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	// Best-effort cleanup of a leftover container from a previous crashed run,
	// so a stale container on the same name/port doesn't mask a real failure.
	_ = exec.Command("docker", "rm", "-f", testContainerName).Run()

	startCmd := exec.Command("docker", "run", "-d",
		"--name", testContainerName,
		"-e", "POSTGRES_USER=garage_test",
		"-e", "POSTGRES_PASSWORD=garage_test",
		"-e", "POSTGRES_DB=garage_test",
		"-p", testPostgresPort+":5432",
		"postgres:16-alpine",
	)
	if out, err := startCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres test container: %v\n%s\n", err, out)
		return 1
	}
	defer func() {
		if out, err := exec.Command("docker", "rm", "-f", testContainerName).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to remove postgres test container: %v\n%s\n", err, out)
		}
	}()

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(testDSN)))
	testDB = bun.NewDB(sqldb, pgdialect.New())
	defer testDB.Close()

	if err := waitForPostgres(testDB, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "postgres test container never became ready: %v\n", err)
		return 1
	}

	if err := createSchema(context.Background(), &testRepository{client: testDB}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test schema: %v\n", err)
		return 1
	}

	return m.Run()
}

func waitForPostgres(db *bun.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return lastErr
}

// newTestStore returns a store backed by the shared test database.
func newTestStore(t *testing.T) *store {
	t.Helper()
	return newStore(&testRepository{client: testDB})
}
