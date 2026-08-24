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

// This file spins up a real, throwaway Postgres container (via `docker run`) for
// the store-level tests in store_test.go (and any RPC-level tests in rpc_test.go
// that need real persistence) to run against, rather than mocking the database —
// the same approach services/tooling/garage/main_test.go already establishes for
// exactly this reason: a real backing store is the more convincing test of
// store.go's queries (jsonb round-tripping, keyset pagination, upsert-on-conflict)
// than a fake ever would be. The container is always torn down, including on a
// failed/panicking test run, via TestMain's defer.

const (
	testContainerName = "bench-workflow-test-postgres"
	testPostgresPort  = "55438" // distinct from garage's 55437, so both suites' containers can run concurrently
	testDSN           = "postgres://bench_test:bench_test@localhost:" + testPostgresPort + "/bench_test?sslmode=disable"
)

// testDB is the shared connection every test in this package uses. Tests isolate
// themselves by using unique run/workflow names rather than by resetting the
// database between tests.
var testDB *bun.DB

// testRepository adapts an already-open *bun.DB into pgbun.Repository, the
// interface store.go and createSchema expect. Its Open is a no-op: the connection
// is already established by TestMain before any test runs.
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
	// Best-effort cleanup of a leftover container from a previous crashed run, so a
	// stale container on the same name/port doesn't mask a real failure.
	_ = exec.Command("docker", "rm", "-f", testContainerName).Run()

	startCmd := exec.Command("docker", "run", "-d",
		"--name", testContainerName,
		"-e", "POSTGRES_USER=bench_test",
		"-e", "POSTGRES_PASSWORD=bench_test",
		"-e", "POSTGRES_DB=bench_test",
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

// newTestStore returns a *pgResultStore backed by the shared test database.
func newTestStore(t *testing.T) *pgResultStore {
	t.Helper()
	return NewPostgresResultStore(&testRepository{client: testDB})
}
