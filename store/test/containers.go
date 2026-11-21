package test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	// Database drivers for connection verification.
	_ "github.com/lib/pq"
)

const (
	testUser     = "root"
	testPassword = "test"

	mysqlNetworkAlias    = "memos-mysql"
	postgresNetworkAlias = "memos-postgres"
)

var (
	mysqlContainer    atomic.Pointer[mysql.MySQLContainer]
	mariadbContainer  atomic.Pointer[mariadb.MariaDBContainer]
	postgresContainer atomic.Pointer[postgres.PostgresContainer]
	mysqlOnce         sync.Once
	postgresOnce      sync.Once
	mysqlBaseDSN      atomic.Value // stores string
	postgresBaseDSN   atomic.Value // stores string
	dbCounter         atomic.Int64
	dbCreationMutex   sync.Mutex // Protects database creation operations

	// Network for container communication.
	testDockerNetwork atomic.Pointer[testcontainers.DockerNetwork]
	testNetworkOnce   sync.Once
)

// getTestNetwork creates or returns the shared Docker network for container communication.
func getTestNetwork(ctx context.Context) (*testcontainers.DockerNetwork, error) {
	var networkErr error
	testNetworkOnce.Do(func() {
		nw, err := network.New(ctx, network.WithDriver("bridge"))
		if err != nil {
			networkErr = err
			return
		}
		testDockerNetwork.Store(nw)
	})
	return testDockerNetwork.Load(), networkErr
}

func requireTestNetwork(ctx context.Context) (*testcontainers.DockerNetwork, error) {
	nw, err := getTestNetwork(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test network")
	}
	if nw == nil {
		return nil, errors.New("test network is unavailable")
	}
	return nw, nil
}

func skipIfContainerProviderUnavailable(t *testing.T) {
	t.Helper()
	if os.Getenv("SKIP_CONTAINER_TESTS") == "1" {
		t.Skip("skipping container-based test (SKIP_CONTAINER_TESTS=1)")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
}

func getMySQLVariant() (string, error) {
	switch variant := os.Getenv("MYSQL_VARIANT"); variant {
	case "", "mysql":
		return "mysql", nil
	case "mariadb":
		return variant, nil
	default:
		return "", errors.Errorf("unsupported MYSQL_VARIANT %q: use mysql or mariadb", variant)
	}
}

// GetMySQLDSN starts the selected MySQL-compatible server and creates a fresh database for this test.
func GetMySQLDSN(t *testing.T) string {
	variant, err := getMySQLVariant()
	if err != nil {
		t.Fatal(err)
	}
	skipIfContainerProviderUnavailable(t)

	ctx := context.Background()

	mysqlOnce.Do(func() {
		nw, err := requireTestNetwork(ctx)
		if err != nil {
			t.Fatalf("failed to create test network: %v", err)
		}

		opts := []testcontainers.ContainerCustomizer{
			testcontainers.WithWaitStrategy(
				wait.ForAll(
					wait.ForLog("ready for connections").WithOccurrence(2),
					wait.ForListeningPort("3306/tcp"),
				).WithDeadline(120 * time.Second),
			),
			network.WithNetwork([]string{mysqlNetworkAlias}, nw),
		}
		var dsn string
		if variant == "mariadb" {
			t.Log("Starting MariaDB container: mariadb:11.4.13")
			container, startErr := mariadb.Run(ctx, "mariadb:11.4.13", append(opts,
				mariadb.WithDatabase("init_db"),
				mariadb.WithUsername(testUser),
				mariadb.WithPassword(testPassword),
				testcontainers.WithCmd("--sql-mode=ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION"),
			)...)
			mariadbContainer.Store(container)
			if startErr != nil {
				t.Fatalf("failed to start MariaDB container: %v", startErr)
			}
			dsn, err = container.ConnectionString(ctx, "multiStatements=true")
		} else {
			t.Log("Starting MySQL container: mysql:8.4")
			container, startErr := mysql.Run(ctx, "mysql:8.4", append(opts,
				mysql.WithDatabase("init_db"),
				mysql.WithUsername(testUser),
				mysql.WithPassword(testPassword),
			)...)
			mysqlContainer.Store(container)
			if startErr != nil {
				t.Fatalf("failed to start MySQL container: %v", startErr)
			}
			dsn, err = container.ConnectionString(ctx, "multiStatements=true")
		}
		if err != nil {
			t.Fatalf("failed to get %s connection string: %v", variant, err)
		}

		if err := waitForDB("mysql", dsn, 30*time.Second); err != nil {
			t.Fatalf("%s not ready for connections: %v", variant, err)
		}

		mysqlBaseDSN.Store(dsn)
	})

	dsn, ok := mysqlBaseDSN.Load().(string)
	if !ok || dsn == "" {
		t.Fatalf("%s container failed to start in a previous test", variant)
	}

	// Serialize database creation to avoid "table already exists" race conditions
	dbCreationMutex.Lock()
	defer dbCreationMutex.Unlock()

	// Create a fresh database for this test
	dbName := fmt.Sprintf("memos_test_%d", dbCounter.Add(1))
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to connect to MySQL: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", dbName)); err != nil {
		t.Fatalf("failed to create database %s: %v", dbName, err)
	}

	// Return DSN pointing to the new database
	return strings.Replace(dsn, "/init_db?", "/"+dbName+"?", 1)
}

// waitForDB polls the database until it's ready or timeout is reached.
func waitForDB(driver, dsn string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return errors.Errorf("timeout waiting for %s database: %v", driver, lastErr)
			}
			return errors.Errorf("timeout waiting for %s database to be ready", driver)
		case <-ticker.C:
			db, err := sql.Open(driver, dsn)
			if err != nil {
				lastErr = err
				continue
			}
			err = db.PingContext(ctx)
			db.Close()
			if err == nil {
				return nil
			}
			lastErr = err
		}
	}
}

// GetPostgresDSN starts a PostgreSQL container (if not already running) and creates a fresh database for this test.
func GetPostgresDSN(t *testing.T) string {
	skipIfContainerProviderUnavailable(t)

	ctx := context.Background()

	postgresOnce.Do(func() {
		nw, err := requireTestNetwork(ctx)
		if err != nil {
			t.Fatalf("failed to create test network: %v", err)
		}

		container, err := postgres.Run(ctx,
			"postgres:18",
			postgres.WithDatabase("init_db"),
			postgres.WithUsername(testUser),
			postgres.WithPassword(testPassword),
			testcontainers.WithWaitStrategy(
				wait.ForAll(
					wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
					wait.ForListeningPort("5432/tcp"),
				).WithDeadline(120*time.Second),
			),
			network.WithNetwork([]string{postgresNetworkAlias}, nw),
		)
		if err != nil {
			t.Fatalf("failed to start PostgreSQL container: %v", err)
		}
		postgresContainer.Store(container)

		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("failed to get PostgreSQL connection string: %v", err)
		}

		if err := waitForDB("postgres", dsn, 30*time.Second); err != nil {
			t.Fatalf("PostgreSQL not ready for connections: %v", err)
		}

		postgresBaseDSN.Store(dsn)
	})

	dsn, ok := postgresBaseDSN.Load().(string)
	if !ok || dsn == "" {
		t.Fatal("PostgreSQL container failed to start in a previous test")
	}

	// Serialize database creation to avoid "table already exists" race conditions
	dbCreationMutex.Lock()
	defer dbCreationMutex.Unlock()

	// Create a fresh database for this test
	dbName := fmt.Sprintf("memos_test_%d", dbCounter.Add(1))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName)); err != nil {
		t.Fatalf("failed to create database %s: %v", dbName, err)
	}

	// Return DSN pointing to the new database
	return strings.Replace(dsn, "/init_db?", "/"+dbName+"?", 1)
}

// TerminateContainers cleans up all running containers and network.
// This is typically called from TestMain.
func TerminateContainers() {
	ctx := context.Background()
	if container := mysqlContainer.Load(); container != nil {
		_ = container.Terminate(ctx)
	}
	if container := mariadbContainer.Load(); container != nil {
		_ = container.Terminate(ctx)
	}
	if container := postgresContainer.Load(); container != nil {
		_ = container.Terminate(ctx)
	}
	if network := testDockerNetwork.Load(); network != nil {
		_ = network.Remove(ctx)
	}
}
