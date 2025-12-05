package txmanager

import (
	"context"

	"gorm.io/gorm"
)

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
