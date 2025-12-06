package txman

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

// You can use testing.T, if you want to test the code without benchmarking
func setupSuite(tb testing.TB) func(tb testing.TB) {
	log.Println("setup suite")

	// Return a function to teardown the test
	return func(tb testing.TB) {
		log.Println("teardown suite")
	}
}

// Almost the same as the above, but this one is for single test instead of collection of tests
func setupTest(tb testing.TB, db *gorm.DB) func(tb testing.TB) {
	log.Println("setup test")

	return func(tb testing.TB) {
		// cleanup table
		log.Println("teardown test")
		db.Model(ItemEntity{}).Delete(&ItemEntity{}, "true")
		//_ = db.Exec("DELETE FROM items")
	}
}

// helper: attach root DB into context using DbCtxKey
func ctxWithRootDB(db *gorm.DB) context.Context {
	return context.WithValue(context.Background(), DbCtxKey{}, db)
}

func Test_TransactionPropagations_MySQL(t *testing.T) {

	teardownSuite := setupSuite(t)
	defer teardownSuite(t)

	endpoint, terminate := startMySQLContainer(t)
	defer func() { _ = terminate(context.Background()) }()

	db, cleanup := openMySQLDB(t, endpoint)
	defer cleanup()
	rootCtx := ctxWithRootDB(db)

	t.Run("Write fail due to ReadOnly flag", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			// inner ReadOnly transaction should not be able to write because of it's parent transaction's ReadOnly level
			return DoInTransaction(ctx, func(ctx context.Context) error {
				txDb2 := MustGetDB(ctx)
				return txDb2.Create(&ItemEntity{Name: "inner"}).Error
			}, TxOptionWithReadonly(false))
		}, TxOptionWithReadonly(true))
		if err == nil {
			t.Fatalf("expected error because of writing in a ReadOnly transaction")
		}

		// transaction must have been rolled back because the ReadOnly flag is true
		if got := countItems(db); got != 0 {
			t.Fatalf("expected 0 committed row, got %d", got)
		}
	})

	t.Run("Write succeed and ignore ReadOnly flag in inner transaction", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			txDb := MustGetDB(ctx)
			if err := txDb.Create(&ItemEntity{Name: "outer"}).Error; err != nil {
				return err
			}

			// inner ReadOnly transaction should be able to write too
			return DoInTransaction(ctx, func(ctx context.Context) error {
				txDb2 := MustGetDB(ctx)
				return txDb2.Create(&ItemEntity{Name: "inner"}).Error
			}, TxOptionWithReadonly(true))
		})
		if err != nil {
			t.Fatalf("expected error because of writing in a ReadOnly transaction")
		}

		// inner must have been rolled back according to outer rollback
		if got := countItems(db); got != 2 {
			t.Fatalf("expected 0 committed row, got %d", got)
		}
	})

	t.Run("REQUIRED rollback all innerly transaction on error", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			txDb := MustGetDB(ctx)
			if err := txDb.Create(&ItemEntity{Name: "outer"}).Error; err != nil {
				return err
			}

			// inner REQUIRES_NEW should commit independently
			err := DoInTransaction(ctx, func(ctx context.Context) error {
				txDb2 := MustGetDB(ctx)
				return txDb2.Create(&ItemEntity{Name: "inner"}).Error
			}, TxOptionWithPropagation(PropagationRequired))

			if err != nil {
				return err
			}

			// force rollback of outer
			return errors.New("force outer rollback")
		}, TxOptionWithPropagation(PropagationRequired))
		if err == nil {
			t.Fatalf("expected outer to return error (forced rollback)")
		}

		// inner must have been rolled back according to outer rollback
		if got := countItems(db); got != 0 {
			t.Fatalf("expected 0 committed row (inner/outer), got %d", got)
		}
	})
	t.Run("REQUIRES_NEW inner commits even when outer rolls back", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			txDb := MustGetDB(ctx)
			if err := txDb.Create(&ItemEntity{Name: "outer"}).Error; err != nil {
				return err
			}

			// inner REQUIRES_NEW should commit independently
			if err := DoInTransaction(ctx, func(ctx context.Context) error {
				txDb2 := MustGetDB(ctx)
				return txDb2.Create(&ItemEntity{Name: "inner"}).Error
			}, TxOptionWithPropagation(PropagationRequiresNew)); err != nil {
				return err
			}

			// force rollback of outer
			return errors.New("force outer rollback")
		})
		if err == nil {
			t.Fatalf("expected outer to return error (forced rollback)")
		}

		// inner must have been committed despite outer rollback
		if got := countItems(db); got != 1 {
			t.Fatalf("expected 1 committed row (inner), got %d", got)
		}
	})

	t.Run("NESTED uses savepoint and rollback does not remove outer changes", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			txDb := MustGetDB(ctx)
			if err := txDb.Create(&ItemEntity{Name: "outer1"}).Error; err != nil {
				return err
			}

			// nested which will fail and rollback to savepoint
			nestedErr := DoInTransaction(ctx, func(ctx context.Context) error {
				txDb2 := MustGetDB(ctx)
				if err := txDb2.Create(&ItemEntity{Name: "nested"}).Error; err != nil {
					return err
				}
				return errors.New("nested fails and must rollback to savepoint")
			}, TxOptionWithPropagation(PropagationNested))

			if nestedErr == nil {
				t.Fatalf("expected nested to return error")
			}

			// continue and commit outer
			return nil
		})
		if err != nil {
			t.Fatalf("outer transaction failed: %v", err)
		}

		// after commit outer1 must exist, nested must NOT
		var names []string
		if err := db.Model(&ItemEntity{}).Pluck("name", &names).Error; err != nil {
			t.Fatalf("pluck names failed: %v", err)
		}
		if len(names) != 1 || names[0] != "outer1" {
			t.Fatalf("unexpected rows after nested rollback, got: %v", names)
		}
	})

	t.Run("SUPPORTS run outside tx when no tx present", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		// SUPPORTS: when no tx, should run non-transactionally
		if err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			txDb := MustGetDB(ctx)
			return txDb.WithContext(ctx).Create(&ItemEntity{Name: "supports"}).Error
		}, TxOptionWithPropagation(PropagationSupports)); err != nil {
			t.Fatalf("supports failed: %v", err)
		}
		if got := countItems(db); got != 1 {
			t.Fatalf("supports should have persisted immediately, got %d", got)
		}
	})

	t.Run("NOT_SUPPORTED run outside tx when no tx present", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		// NOT_SUPPORTED: when not in tx should behave similarly (run without tx)
		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			txDb := MustGetDB(ctx)
			return txDb.WithContext(ctx).Create(&ItemEntity{Name: "not_supported"}).Error
		}, TxOptionWithPropagation(PropagationNotSupported))

		if err != nil {
			t.Fatalf("not_supported failed: %v", err)
		}
		if got := countItems(db); got != 1 {
			t.Fatalf("not_supported should have persisted immediately, got %d", got)
		}
	})

	t.Run("MANDATORY without tx fails", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		// MANDATORY without a tx should return ErrNoTransaction
		err := DoInTransaction(rootCtx, func(ctx context.Context) error { return nil }, TxOptionWithPropagation(PropagationMandatory))
		if !errors.Is(err, ErrNoTransaction) {
			t.Fatalf("expected ErrNoTransaction from MANDATORY without tx, got %v", err)
		}
	})

	t.Run("MANDATORY inside tx success", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			// MANDATORY in a tx should succeed
			return DoInTransaction(ctx, func(ctx context.Context) error {
				txDb2 := MustGetDB(ctx)
				return txDb2.WithContext(ctx).Create(&ItemEntity{Name: "mandatory"}).Error
			}, TxOptionWithPropagation(PropagationMandatory))
		})
		if errors.Is(err, ErrNoTransaction) {
			t.Fatalf("expected no ErrNoTransaction error for MANDATORY because it runs in a transaction, got %v", err)
		}
		if err != nil {
			t.Fatalf("mandatory failed: %v", err)
		}
		if got := countItems(db); got != 1 {
			t.Fatalf("mandatory should have persisted immediately, got %d", got)
		}
	})

	t.Run("NEVER inside tx fails", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		// NEVER inside a transaction should return ErrTransactionPresent.
		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			err := DoInTransaction(ctx, func(ctx context.Context) error { return nil }, TxOptionWithPropagation(PropagationNever))
			if !errors.Is(err, ErrTransactionPresent) {
				return errors.New("expected ErrTransactionPresent from NEVER inside tx")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("outer REQUIRED failed: %v", err)
		}
	})

	t.Run("NEVER successful run without any transaction", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		// without outer tx -> attempt NEVER should succeed
		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			txDb := MustGetDB(ctx)
			return txDb.WithContext(ctx).Create(&ItemEntity{Name: "never"}).Error
		}, TxOptionWithPropagation(PropagationNever))

		if err != nil {
			t.Fatalf("expected ErrTransactionPresent from NEVER inside tx")
		}

		if got := countItems(db); got != 1 {
			t.Fatalf("never should have persisted immediately, got %d", got)
		}
	})

	t.Run("post-commit hooks execute once and capture panics", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		var ran int32
		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			hc, _ := ctx.Value(TxHooksKey{}).(*HooksContainer)
			if hc == nil {
				t.Fatalf("hooks container not found in ctx")
			}
			hc.addHook(func(ctx context.Context) error {
				ran = 1
				return nil
			})
			// hook that panics (should be captured and not break commit)
			hc.addHook(func(ctx context.Context) error {
				panic("boom")
			})
			// insert and commit
			txDb := MustGetDB(ctx)
			return txDb.WithContext(ctx).Create(&ItemEntity{Name: "required"}).Error
		})
		if err != nil {
			t.Fatalf("transaction failed: %v", err)
		}

		// small wait for hooks to run (hooks run synchronously after commit in this impl,
		// but keep tiny margin)
		time.Sleep(50 * time.Millisecond)
		if ran != 1 {
			t.Fatalf("post-commit hook didn't run or panicked: ran=%d", ran)
		}
	})

	t.Run("AfterCommit hooks once the transaction commited", func(t *testing.T) {

		teardownTest := setupTest(t, db)
		defer teardownTest(t)

		var ran int32
		err := DoInTransaction(rootCtx, func(ctx context.Context) error {
			AfterCommit(ctx, func(ctx context.Context) error {
				atomic.StoreInt32(&ran, 1)
				return nil
			})
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
	})
}
