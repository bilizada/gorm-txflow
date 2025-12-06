package txflow

import (
	"context"

	"gorm.io/gorm"
)

type GormDB func(ctx context.Context) *gorm.DB

func CreateGormDB(db *gorm.DB) GormDB {
	return func(ctx context.Context) *gorm.DB {
		if db == nil {
			return nil
		}
		// if no context provided, return the base DB (don't call WithContext with nil)
		if ctx == nil {
			return db
		}
		// if context carries a DB, prefer it
		if ctxDb, ok := GetDB(ctx); ok && ctxDb != nil {
			return ctxDb.WithContext(ctx)
		}
		return db.WithContext(ctx)
	}
}
