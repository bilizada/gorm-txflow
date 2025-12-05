package txmanager

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ------------------------------------------------------------------
// Context keys
// ------------------------------------------------------------------

type DbCtxKey struct{}   // root *gorm.DB in request/context
type TxDBKey struct{}    // active tx *gorm.DB (stored in ctx while inside tx)
type TxHooksKey struct{} // HooksContainer associated with logical transaction

// ------------------------------------------------------------------
// Hook types & container
// ------------------------------------------------------------------

// TxHookFunc runs after the associated transaction commits successfully.
// It may return an error; those errors are reported to a handler configured
// on the plugin.
type TxHookFunc func(ctx context.Context) error

// HooksContainer contains the hook functions to execute according to transaction lifecycle
type HooksContainer struct {
	mu       sync.Mutex
	hooks    []TxHookFunc
	executed bool
}

// addHook adds a new hook function to the HooksContainer
func (hc *HooksContainer) addHook(h TxHookFunc) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if hc.executed {
		// defensive: ignore hooks added after execution
		return
	}
	hc.hooks = append(hc.hooks, h)
}

// Execute runs all hooks once and returns slice of errors (including converted panics).
func (hc *HooksContainer) Execute(ctx context.Context) []error {
	hc.mu.Lock()
	if hc.executed {
		hc.mu.Unlock()
		return nil
	}
	hooks := hc.hooks
	hc.hooks = nil
	hc.executed = true
	hc.mu.Unlock()

	var errs []error
	for _, h := range hooks {
		func(hf TxHookFunc) {
			defer func() {
				if r := recover(); r != nil {
					errs = append(errs, fmt.Errorf("panic in post-commit hook: %v\nStack Trace:\n%s", r, string(debug.Stack())))
				}
			}()
			if err := hf(ctx); err != nil {
				errs = append(errs, err)
			}
		}(h)
	}
	return errs
}

// DoInTransaction runs fn under given propagation, reading the root DB from ctx.
// fn is func(ctx context.Context) error — the tx-aware context will be passed.
// Use TxManagerHttpMiddleware or WithDB to attach the root DB into the context beforehand.
func DoInTransaction(ctx context.Context, fn func(ctx context.Context) error, txOptions ...TxOption) error {
	txOption, err := mergeOptions(txOptions...)
	if err != nil {
		return err
	}

	// resolve DB
	db, ok := GetDB(ctx)
	if !ok || db == nil {
		return ErrNoDBAwareContext
	}

	switch *txOption.propagation {
	case PropagationRequired:
		if tx := ctxTx(ctx); tx != nil {
			// already in tx: pass the same ctx (should be tx-aware) to fn
			return fn(ctx)
		}
		return beginNewTransactionWithHooks(ctx, db, fn)

	case PropagationRequiresNew:
		return beginNewTransactionWithHooksOnNewSession(ctx, db, fn)

	case PropagationSupports:
		if tx := ctxTx(ctx); tx != nil {
			return fn(ctx)
		}
		return fn(db.WithContext(ctx).Statement.Context)

	case PropagationNotSupported:
		if ctxTx(ctx) != nil {
			newDB := db.Session(&gorm.Session{NewDB: true})
			return fn(newDB.WithContext(ctx).Statement.Context)
		}
		return fn(db.WithContext(ctx).Statement.Context)

	case PropagationMandatory:
		if tx := ctxTx(ctx); tx != nil {
			return fn(ctx)
		}
		return ErrNoTransaction

	case PropagationNever:
		if tx := ctxTx(ctx); tx != nil {
			return ErrTransactionPresent
		}
		return fn(db.WithContext(ctx).Statement.Context)

	case PropagationNested:
		if tx := ctxTx(ctx); tx == nil {
			return beginNewTransactionWithHooks(ctx, db, fn)
		}
		return runNestedUsingSavepoint(ctx, db, fn)

	default:
		return ErrMultiplePropagation
	}
}

// ctxTx gets the active tx *gorm.DB stored in ctx (TxDBKey).
func ctxTx(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return nil
	}
	if tx, _ := ctx.Value(TxDBKey{}).(*gorm.DB); tx != nil {
		return tx
	}
	return nil
}

// beginNewTransactionWithHooks starts a new DB transaction and attaches a HooksContainer and tx pointer to the context.
// fn receives tx.Statement.Context (the context carrying hooks and tx pointer).
func beginNewTransactionWithHooks(baseCtx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	hc := &HooksContainer{}
	ctxWithHooks := context.WithValue(baseCtx, TxHooksKey{}, hc)

	// Run the GORM transaction using the ctx that contains the hooks container.
	// When db.DoInTransaction returns nil, the transaction committed successfully.
	err := db.WithContext(ctxWithHooks).Transaction(func(tx *gorm.DB) error {
		// attach pointer to tx to the context so nested code can locate it
		ctxWithHooksAndTx := context.WithValue(ctxWithHooks, TxDBKey{}, tx)
		// ensure tx.Statement.Context is set to ctxWithHooksAndTx
		tx = tx.WithContext(ctxWithHooksAndTx)

		// run user fn
		if err := fn(tx.Statement.Context); err != nil {
			return err // rollback
		}
		return nil // commit
	})

	// If commit succeeded (err == nil), execute hooks now using ctxWithHooks (or another context you prefer).
	if err == nil {
		// run hooks and handle errors (here using default handler; adapt if you have a custom handler)
		if errs := hc.Execute(ctxWithHooks); len(errs) > 0 {
			for _, e := range errs {
				// You can log, or call a configured handler instead of using log.Printf.
				log.Printf("post-commit hook error: %v", e)
			}
		}
	}
	return err
}

// beginNewTransactionWithHooksOnNewSession starts a fresh DB session and transaction (REQUIRES_NEW).
func beginNewTransactionWithHooksOnNewSession(baseCtx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	hc := &HooksContainer{}
	ctxWithHooks := context.WithValue(baseCtx, TxHooksKey{}, hc)

	newDB := db.Session(&gorm.Session{NewDB: true})
	err := newDB.WithContext(ctxWithHooks).Transaction(func(tx *gorm.DB) error {
		ctxWithHooksAndTx := context.WithValue(ctxWithHooks, TxDBKey{}, tx)
		tx = tx.WithContext(ctxWithHooksAndTx)

		if err := fn(tx.Statement.Context); err != nil {
			return err
		}
		return nil
	})

	// execute hooks only if commit succeeded
	if err == nil {
		if errs := hc.Execute(ctxWithHooks); len(errs) > 0 {
			for _, e := range errs {
				log.Printf("post-commit hook error (requires_new): %v", e)
			}
		}
	}
	return err
}

// runNestedUsingSavepoint runs fn inside an existing transaction using savepoints (NESTED).
// Hooks registered during nested execution attach to top-level HooksContainer so they only run on outer commit.
func runNestedUsingSavepoint(baseCtx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	// locate current tx from context (expect it to be present in ctx)
	tx := ctxTx(baseCtx)
	if tx == nil {
		// defensive fallback: start a new tx
		return beginNewTransactionWithHooks(baseCtx, db, fn)
	}

	spName := generateSavepointName()
	if err := tx.SavePoint(spName).Error; err != nil {
		return fmt.Errorf("failed to create savepoint: %w", err)
	}

	// Call the nested function using the same tx-aware context (hooks attach to top-level hooks container).
	// Use baseCtx because it should be the tx-aware context provided to the nested call.
	if err := fn(baseCtx); err != nil {
		if rbErr := tx.RollbackTo(spName).Error; rbErr != nil {
			return fmt.Errorf("nested rollback failed: %v (rollback error: %v)", err, rbErr)
		}
		return err
	}
	// success => nothing else required (savepoint release optional on many DBs)
	return nil
}

func generateSavepointName() string {
	return fmt.Sprintf("sp_%d", time.Now().UnixNano())
}
