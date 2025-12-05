package txman

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ItemEntity model for tests
type ItemEntity struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func startMySQLContainer(t *testing.T) (endpoint string, terminate func(ctx context.Context, opts ...testcontainers.TerminateOption) error) {
	t.Helper()
	ctx := context.Background()

	container, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("testdb"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("port: 3306  MySQL Community Server - GPL").WithStartupTimeout(2*time.Minute),
		))
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("get host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "3306")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("get mapped port: %v", err)
	}

	endpoint = fmt.Sprintf("%s:%s", host, mappedPort.Port())
	return endpoint, container.Terminate
}

func openMySQLDB(t *testing.T, endpoint string) (*gorm.DB, func()) {
	t.Helper()
	dsn := fmt.Sprintf("root:pass@tcp(%s)/testdb?charset=utf8mb4&parseTime=True&loc=Local", endpoint)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open mysql gorm: %v", err)
	}
	// ensure table exists
	if err := db.AutoMigrate(&ItemEntity{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db, func() {}
}

func openMemoryDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	dsn := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory: %v", err)
	}
	if err := db.AutoMigrate(&ItemEntity{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db, func() {}
}

func openFileBackedSQLite(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, fmt.Sprintf("testdb.%d.sqlite", time.Now().UnixNano()))
	dsn := fmt.Sprintf("file:%s?cache=shared&_journal_mode=WAL&_foreign_keys=1", path)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite file-backed DB: %v", err)
	}

	// ensure PRAGMAs applied on connection pool
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("getting sql.DB: %v", err)
	}
	// optional: set max open connections
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(5)

	// run a PRAGMA to ensure WAL is set (driver may ignore query params)
	if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		t.Fatalf("set journal_mode WAL: %v", err)
	}
	if err := db.AutoMigrate(&ItemEntity{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
		_ = os.Remove(path)
	}
	return db, cleanup
}
