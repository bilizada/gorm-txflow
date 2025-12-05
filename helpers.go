package txmanager

import (
	"context"

	"gorm.io/gorm"
)

// AfterCommit registers a post-commit hook inside an active transaction context.
// Panics if called outside a transactional context (no HooksContainer found).
func AfterCommit(ctx context.Context, hook TxHookFunc) {
	if ctx == nil {
		panic("AfterCommit: context is nil; must be called inside a transaction")
	}
	if hc, _ := ctx.Value(TxHooksKey{}).(*HooksContainer); hc != nil {
		hc.addHook(hook)
		return
	}
	panic(ErrNoTransaction)
}

func WithDB(ctx context.Context, db *gorm.DB) context.Context {
	_, ok := GetDB(ctx)
	if ok {
		return ctx
	}
	return context.WithValue(ctx, DbCtxKey{}, db)
}

// GetDB returns the root *gorm.DB stored in ctx.
func GetDB(ctx context.Context) (*gorm.DB, bool) {
	if ctx == nil {
		return nil, false
	}
	if tx, _ := ctx.Value(TxDBKey{}).(*gorm.DB); tx != nil {
		return tx, true
	}
	db, ok := ctx.Value(DbCtxKey{}).(*gorm.DB)
	return db, ok
}

// MustGetDB returns the root *gorm.DB or panics.
func MustGetDB(ctx context.Context) *gorm.DB {
	if db, ok := GetDB(ctx); ok && db != nil {
		return db
	}
	panic(ErrNoDBAwareContext)
}

// GetTx returns the active transaction *gorm.DB stored in ctx, if present.
func GetTx(ctx context.Context) (*gorm.DB, bool) {
	if ctx == nil {
		return nil, false
	}
	if tx, _ := ctx.Value(TxDBKey{}).(*gorm.DB); tx != nil {
		return tx, true
	}
	return nil, false
}

// MustGetTx returns the active transaction *gorm.DB or panics if not present.
func MustGetTx(ctx context.Context) *gorm.DB {
	if tx, ok := GetTx(ctx); ok && tx != nil {
		return tx
	}
	panic(ErrNoTransaction)
}
