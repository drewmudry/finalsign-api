package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// mustStartPostgresContainer starts a postgres container and returns a teardown function,
// a connection string, and an error.
func mustStartPostgresContainer() (func(context.Context, ...testcontainers.TerminateOption) error, string, error) {
	var (
		dbName = "test_db" // Changed to avoid potential conflict with a real "database"
		dbPwd  = "password"
		dbUser = "user"
	)

	dbContainer, err := postgres.Run(
		context.Background(),
		"postgres:latest",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPwd),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to start postgres container: %w", err)
	}

	host, err := dbContainer.Host(context.Background())
	if err != nil {
		return dbContainer.Terminate, "", fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := dbContainer.MappedPort(context.Background(), "5432/tcp")
	if err != nil {
		return dbContainer.Terminate, "", fmt.Errorf("failed to get container mapped port: %w", err)
	}

	connStr := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPwd, host, port.Port(), dbName)

	return dbContainer.Terminate, connStr, nil
}

func TestMain(m *testing.M) {
	teardown, testDbString, err := mustStartPostgresContainer()
	if err != nil {
		log.Fatalf("could not start postgres container for tests: %v", err)
	}

	originalDbString := os.Getenv("DB_STRING")
	if err := os.Setenv("DB_STRING", testDbString); err != nil {
		log.Fatalf("failed to set DB_STRING for tests: %v", err)
	}
	// Ensure dbInstance is reset if tests are run multiple times in a more complex scenario
	// For a single 'go test' run, it starts as nil.
	dbInstance = nil

	exitCode := m.Run()

	// Restore original DB_STRING
	if originalDbString == "" {
		os.Unsetenv("DB_STRING")
	} else {
		if err := os.Setenv("DB_STRING", originalDbString); err != nil {
			log.Printf("warning: failed to restore original DB_STRING: %v", err)
		}
	}

	// Teardown the container
	if teardown != nil {
		if err := teardown(context.Background()); err != nil {
			log.Fatalf("could not teardown postgres container: %v", err)
		}
	}
	os.Exit(exitCode)
}

func TestNew(t *testing.T) {
	// dbInstance should be nil here or point to the test DB
	// due to TestMain setup and the singleton nature of New()
	srv := New()
	if srv == nil {
		t.Fatal("New() returned nil")
	}
	// Ensure we close the connection that New() might have opened.
	// This is important if each test function gets a truly fresh dbInstance.
	// However, with the current singleton, this Close() will affect other tests.
	// For the current structure where TestMain manages one dbInstance for all tests,
	// this specific close might be problematic.
	// dbInstance = nil // Reset for the next test if needed, or manage in TestMain
}

func TestHealth(t *testing.T) {
	srv := New()    // Relies on TestMain having set up DB_STRING for a test DB
	if srv == nil { // Should not happen if New() is robust
		t.Fatal("New() returned nil before health check")
		return
	}

	stats := srv.Health()

	if stats["status"] != "up" {
		t.Fatalf("expected status to be up, got %s (error: %s)", stats["status"], stats["error"])
	}

	if errMsg, ok := stats["error"]; ok {
		t.Fatalf("expected error not to be present, got: %s", errMsg)
	}

	// The default "It's healthy" message might be overridden by specific stats checks in Health()
	// So, we primarily check for "up" and no error.
	// if stats["message"] != "It's healthy" {
	// 	t.Fatalf("expected message to be 'It's healthy' or a specific load message, got %s", stats["message"])
	// }
}

func TestClose(t *testing.T) {
	// This test is tricky with the singleton dbInstance managed by TestMain.
	// If this runs, it closes the connection for subsequent tests.
	// A better approach might be for New() to return a closable service
	// and each test manages its own lifecycle if true isolation per test is needed.
	// For now, assuming TestMain provides a single shared instance for the test suite.

	// srv := New() // Get the shared instance
	// if err := srv.Close(); err != nil {
	// 	t.Fatalf("expected Close() to return nil, got %v", err)
	// }
	// dbInstance = nil // Mark as closed so a subsequent New() would re-initialize
	// This is important because other tests might call New() again.
	// Or, perhaps TestClose should be the last test, or TestMain should handle
	// closing the main test dbInstance.

	// Given the current structure, let's test if calling Close on the shared instance works.
	// The actual dbInstance is closed by TestMain's teardown implicitly when the process exits
	// or if we explicitly call dbInstance.Close() in TestMain before teardown.
	// For now, let's assume TestMain's teardown handles the container, and db connection closes with it.
	// A specific TestClose might be redundant or interfere if not handled carefully.
	// Let's make it simple: create a service, close it, and ensure dbInstance is reset.
	// This implies tests after this one might get a fresh connection if they call New().

	currentInstance := dbInstance // Capture before New()
	srv := New()                  // Should return the existing test instance
	if srv == nil {
		t.Fatal("New() returned nil in TestClose")
		return
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	// After closing, the dbInstance in database.go should ideally be nil or its db connection unusable.
	// Our New() function reuses dbInstance if not nil.
	// To ensure a subsequent New() call in another test would get a fresh connection,
	// dbInstance itself (the pointer in database.go) should be set to nil.
	if dbInstance == currentInstance && dbInstance != nil {
		// If the global dbInstance is still the same and not nil, then New() would return it,
		// but its underlying connection is closed. Ping should fail.
		if err := dbInstance.db.Ping(); err == nil {
			t.Error("Expected ping to fail on a closed connection, but it succeeded.")
		}
	}
	dbInstance = nil // Manually reset for subsequent tests. This is a must.
}

// A helper to run migrations on the test database
func runTestMigrations(tb testing.TB, dbService Service) {
	// The Service interface doesn't have RunMigrations.
	// We need to cast to the concrete type `service` or add RunMigrations to the interface.
	// For now, let's assume we can get the *sql.DB instance or RunMigrations is accessible.

	// This part needs adjustment based on how migrations are actually run.
	// If RunMigrations is part of the `service` struct and not the `Service` interface:
	s, ok := dbService.(*service)
	if !ok {
		tb.Fatal("Failed to cast Service to *service to run migrations")
		return
	}

	// Temporarily adjust migrate.go to use DB_STRING or pass *sql.DB if needed.
	// For simplicity, if migrate.go's RunMigrations can be called on a *service instance,
	// and if it uses s.db which is set up by New() with the test DB_STRING:

	// Need to ensure migrate.go's file paths are correct for tests.
	// Often, migrations are in a subfolder, and 'file://migrations' path needs to be relative
	// to the test execution directory or an absolute path.
	// This might require a test-specific migration setup or configuration.

	// Assuming RunMigrations() is a method on *service:
	err := s.RunMigrations() // This method is defined in migrate.go
	if err != nil {
		tb.Fatalf("Failed to run migrations on test database: %v", err)
	}
}

// Example of a test that needs migrations:
func TestCreateOrUpdateUser_WithMigrations(t *testing.T) {
	// dbInstance should be nil here for a fresh start for this specific test function's
	// scope if we were to reset it before each test function.
	// However, with TestMain, it's a shared instance.
	dbInstance = nil // Reset before this test to ensure New() picks up DB_STRING.
	// And to get a fresh connection for this test.

	srv := New()
	if srv == nil {
		t.Fatal("New() returned nil")
	}

	// Run migrations for this test
	runTestMigrations(t, srv)

	user := &User{
		Provider:   "google",
		ProviderID: "test_provider_id_123",
		Email:      "test@example.com",
		Name:       "Test User",
		AvatarURL:  "http://example.com/avatar.jpg",
	}

	err := srv.CreateOrUpdateUser(user)
	if err != nil {
		t.Fatalf("CreateOrUpdateUser failed: %v", err)
	}

	if user.ID == 0 {
		t.Error("Expected user ID to be populated, got 0")
	}
	if user.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be populated")
	}
	if user.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be populated")
	}

	// Clean up: Reset dbInstance so other tests don't use a connection that might have
	// tables created by this test's migrations if migrations are not idempotent or
	// if there's no per-test DB cleanup.
	// If TestClose runs after this, it will handle its own dbInstance reset.
	// This manual reset is becoming fragile.
	// A better pattern is often table-truncation between tests or per-test transactions that rollback.
	dbInstance = nil
}

// TODO: Add TestGetUserByProviderID, TestGetUserByEmail, TestGetUserByID
// These tests will also need migrations to be run first.
// Consider a test suite structure (e.g., using t.Run with subtests) where migrations
// are run once for the suite.
