package txmanager

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// simple model for DB operations
type Item struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func openMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in-memory: %v", err)
	}
	if err := db.AutoMigrate(&Item{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func countItems(db *gorm.DB) int64 {
	var c int64
	_ = db.Model(&Item{}).Count(&c)
	return c
}

// attach DB into context using the package key (DbCtxKey{})
func ctxWithDB(db *gorm.DB) context.Context {
	return WithDB(context.Background(), db)
}

func Test_DoInTransaction_RequiresNew_CommitsWhenOuterRollsBack(t *testing.T) {
	db := openMemoryDB(t)
	ctx := ctxWithDB(db)

	// Outer transaction (default = REQUIRED).
	err := DoInTransaction(ctx, func(ctx context.Context) error {
		// outer insert (should be rolled back)
		txDb, _ := GetDB(ctx)
		if txDb == nil {
			t.Fatalf("expected tx DB in outer")
		}
		if err := txDb.Create(&Item{Name: "outer"}).Error; err != nil {
			return err
		}

		// inner REQUIRES_NEW should commit independently
		if err := DoInTransaction(ctx, func(ctx context.Context) error {
			txDb2, _ := GetDB(ctx)
			if txDb2 == nil {
				t.Fatalf("expected tx DB in inner requires_new")
			}
			return txDb2.Create(&Item{Name: "inner"}).Error
		}, TxOptionWithPropagation(PropagationRequiresNew)); err != nil {
			return err
		}

		// force rollback of outer
		return errors.New("force outer rollback")
	})
	// outer returned error => overall should be error
	if err == nil {
		t.Fatalf("expected outer to return error (forced rollback)")
	}

	// inner must have been committed despite outer rollback
	if got := countItems(db); got != 1 {
		t.Fatalf("expected 1 committed row (inner), got %d", got)
	}
}

func Test_DoInTransaction_Nested_SavepointRollback(t *testing.T) {
	db := openMemoryDB(t)
	ctx := ctxWithDB(db)

	err := DoInTransaction(ctx, func(ctx context.Context) error {
		txDb, _ := GetDB(ctx)
		if txDb == nil {
			t.Fatalf("expected tx DB in outer")
		}
		if err := txDb.Create(&Item{Name: "outer1"}).Error; err != nil {
			return err
		}

		// run nested which returns an error; outer will catch it and continue
		nestedErr := DoInTransaction(ctx, func(ctx context.Context) error {
			txDb2, _ := GetDB(ctx)
			if txDb2 == nil {
				t.Fatalf("expected tx DB in nested")
			}
			if err := txDb2.Create(&Item{Name: "nested"}).Error; err != nil {
				return err
			}
			return errors.New("nested fails and must rollback to savepoint")
		}, TxOptionWithPropagation(PropagationNested))

		// nested call should return an error (we expect that) but outer should ignore it to continue
		if nestedErr == nil {
			t.Fatalf("expected nested to return error")
		}

		// continue and commit outer
		return nil
	})
	if err != nil {
		t.Fatalf("outer transaction failed: %v", err)
	}

	// after commit outer1 must exist, nested must NOT (rolled back by savepoint)
	var names []string
	_ = db.Model(&Item{}).Pluck("name", &names)
	if len(names) != 1 || names[0] != "outer1" {
		t.Fatalf("unexpected rows after nested rollback, got: %v", names)
	}
}

func Test_DoInTransaction_Supports_NotSupported_Behaviour(t *testing.T) {
	db := openMemoryDB(t)
	ctx := ctxWithDB(db)

	// SUPPORTS with no tx => should run non-transactionally and persist immediately
	if err := DoInTransaction(ctx, func(ctx context.Context) error {
		d, _ := GetDB(ctx)
		if d == nil {
			// DoInTransaction for SUPPORTS when no tx passes a non-tx context (db.WithContext(ctx).Statement.Context)
			// but GetDB(ctx) might be nil; still we can operate using the root DB
			return db.WithContext(ctx).Create(&Item{Name: "supports"}).Error
		}
		return d.Create(&Item{Name: "supports"}).Error
	}, TxOptionWithPropagation(PropagationSupports)); err != nil {
		t.Fatalf("supports failed: %v", err)
	}
	if got := countItems(db); got != 1 {
		t.Fatalf("supports should have persisted immediately, got %d", got)
	}

	// NOT_SUPPORTED: when not in tx should behave similarly (run without tx)
	if err := DoInTransaction(ctx, func(ctx context.Context) error {
		return db.WithContext(ctx).Create(&Item{Name: "not_supported"}).Error
	}, TxOptionWithPropagation(PropagationNotSupported)); err != nil {
		t.Fatalf("not_supported failed: %v", err)
	}
	if got := countItems(db); got != 2 {
		t.Fatalf("not_supported should have persisted immediately, got %d", got)
	}
}

func Test_DoInTransaction_MandatoryAndNever(t *testing.T) {
	db := openMemoryDB(t)
	ctx := ctxWithDB(db)

	// MANDATORY without a tx should return ErrNoTransaction
	if err := DoInTransaction(ctx, func(ctx context.Context) error {
		return nil
	}, TxOptionWithPropagation(PropagationMandatory)); err != ErrNoTransaction {
		t.Fatalf("expected ErrNoTransaction from MANDATORY without tx, got %v", err)
	}

	// NEVER inside a transaction should return ErrTransactionPresent.
	err := DoInTransaction(ctx, func(ctx context.Context) error {
		// inside outer tx -> attempt NEVER should fail
		if err := DoInTransaction(ctx, func(ctx context.Context) error { return nil }, TxOptionWithPropagation(PropagationNever)); err != ErrTransactionPresent {
			return errors.New("expected ErrTransactionPresent from NEVER inside tx")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("outer REQUIRED failed: %v", err)
	}
}

func Test_PostCommitHooksExecuteOnce(t *testing.T) {
	db := openMemoryDB(t)
	ctx := ctxWithDB(db)

	var ran int32
	err := DoInTransaction(ctx, func(ctx context.Context) error {
		// get hooks container from ctx (it is attached in beginNewTransactionWithHooks)
		hc, _ := ctx.Value(TxHooksKey{}).(*HooksContainer)
		if hc == nil {
			t.Fatalf("hooks container not found in ctx")
		}
		hc.addHook(func(ctx context.Context) error {
			atomic.StoreInt32(&ran, 1)
			return nil
		})
		// add a hook that panics to ensure Execute captures panics (shouldn't fail the commit)
		hc.addHook(func(ctx context.Context) error {
			panic("boom")
		})
		// commit
		return nil
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
	// hooks executed after commit (allow tiny margin)
	time.Sleep(10 * time.Millisecond)
	if atomic.LoadInt32(&ran) != 1 {
		t.Fatalf("post-commit hook didn't run")
	}
}
