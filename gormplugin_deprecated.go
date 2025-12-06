package txflow

import (
	"context"
	"log"

	"gorm.io/gorm"
)

// ------------------------------------------------------------------
// GORM plugin: run hooks after commit and route hook errors to handler
// ------------------------------------------------------------------

// HookErrorHandler is invoked for each hook error returned by HooksContainer.Execute.
type HookErrorHandler func(ctx context.Context, err error)

type plugin struct {
	onHookError HookErrorHandler
}

// NewTransactionManagerPlugin returns plugin that logs hook errors with the default logger.
func NewTransactionManagerPlugin() gorm.Plugin {
	return NewPluginWithHandler(defaultHookErrorHandler)
}

// NewPluginWithHandler returns plugin with a custom hook error handler.
func NewPluginWithHandler(handler HookErrorHandler) gorm.Plugin {
	if handler == nil {
		handler = defaultHookErrorHandler
	}
	return &plugin{onHookError: handler}
}

func (p *plugin) Name() string {
	return "txprop_hooks_plugin"
}

func (p *plugin) Initialize(db *gorm.DB) error {
	cb := func(db *gorm.DB) {
		if db == nil || db.Statement == nil || db.Statement.Context == nil {
			return
		}
		// Only execute hooks when operation succeeded (commit succeeded)
		if db.Error != nil {
			return
		}
		if hc, _ := db.Statement.Context.Value(TxHooksKey{}).(*HooksContainer); hc != nil {
			if errs := hc.Execute(db.Statement.Context); len(errs) > 0 {
				for _, e := range errs {
					func(err error) {
						// protect handler from panics
						defer func() { _ = recover() }()
						p.onHookError(db.Statement.Context, err)
					}(e)
				}
			}
		}
	}

	// Register on common callback chains. Using After("gorm:commit_or_rollback_transaction")
	// so callbacks run at the end of transaction life-cycle.
	_ = db.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("txprop:after_create", cb)
	_ = db.Callback().Update().After("gorm:commit_or_rollback_transaction").Register("txprop:after_update", cb)
	_ = db.Callback().Delete().After("gorm:commit_or_rollback_transaction").Register("txprop:after_delete", cb)
	_ = db.Callback().Row().After("gorm:commit_or_rollback_transaction").Register("txprop:after_row", cb)
	_ = db.Callback().Raw().After("gorm:commit_or_rollback_transaction").Register("txprop:after_raw", cb)

	return nil
}

func defaultHookErrorHandler(ctx context.Context, err error) {
	log.Printf("txprop: post-commit hook error: %v", err)
}
