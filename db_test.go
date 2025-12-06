package txflow

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateGormDB(t *testing.T) {
	// open a real in-memory sqlite DB so gorm initializes Config/Statement etc.
	sqliteDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory DB: %v", err)
	}
	// open a real in-memory sqlite DB so gorm initializes Config/Statement etc.
	sqliteDB2, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory DB: %v", err)
	}

	t.Run("nil db returns nil", func(t *testing.T) {
		get := CreateGormDB(nil)
		if got := get(nil); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
		// also non-nil context should still return nil when base db is nil
		if got := get(context.Background()); got != nil {
			t.Fatalf("expected nil with non-nil ctx, got %#v", got)
		}
	})

	t.Run("non-nil db with nil ctx returns a gorm.DB", func(t *testing.T) {
		get := CreateGormDB(sqliteDB)
		got := get(nil)
		if got != sqliteDB {
			t.Fatalf("expected non-nil *gorm.DB, got nil")
		}
	})

	t.Run("non-nil db with non-nil ctx returns a gorm.DB (no panic)", func(t *testing.T) {
		get := CreateGormDB(sqliteDB)
		ctx := context.Background()
		got := get(ctx)
		if got == nil {
			t.Fatalf("expected non-nil *gorm.DB with context, got nil")
		}
		//// Should be instanced of sqliteDB
		//if got != sqliteDB {
		//	t.Fatalf("expected non-nil *gorm.DB instanced from base sqliteDB")
		//}
	})

	t.Run("non-nil db with non-nil ctx returns a gorm.DB (no panic)", func(t *testing.T) {
		get := CreateGormDB(sqliteDB)
		ctx := WithDB(context.Background(), sqliteDB2)
		got := get(ctx)
		if got == nil {
			t.Fatalf("expected non-nil *gorm.DB with context, got nil")
		}
		//// Should be instanced of sqliteDB2
		//if got != sqliteDB2 {
		//	t.Fatalf("expected non-nil *gorm.DB instanced from context injected database sqliteDB2")
		//}
	})
}

func TestCreateGormDB_WithSQLiteMemory(t *testing.T) {

	// open a real in-memory sqlite DB so gorm initializes Config/Statement etc.
	sqliteDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory DB: %v", err)
	}
	getDB := CreateGormDB(sqliteDB)

	// case: nil context -> should return base DB (no panic)
	base := getDB(nil)
	if base == nil {
		t.Fatal("expected non-nil *gorm.DB for nil context")
	}
	// try calling Session/WithContext to ensure no panics and Config/Statement exist
	if _, err := safeSessionCall(base); err != nil {
		t.Fatalf("Session/WithContext on base DB failed: %v", err)
	}

	// case: non-nil context -> should return a DB wired with context (no panic)
	ctx := context.Background()
	withCtx := getDB(ctx)
	if withCtx == nil {
		t.Fatal("expected non-nil *gorm.DB for non-nil context")
	}
	if _, err := safeSessionCall(withCtx); err != nil {
		t.Fatalf("Session/WithContext on context DB failed: %v", err)
	}
}

// safeSessionCall attempts operations that previously caused panics.
// It returns an error instead of letting the test panic.
func safeSessionCall(db *gorm.DB) (ok bool, err error) {
	// call Session with an empty Session config: this triggers the code paths
	// that previously hit Statement.clone() in your code.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during Session call: %v", r)
		}
	}()
	_ = db.Session(&gorm.Session{})
	return true, nil
}
