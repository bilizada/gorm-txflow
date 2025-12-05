package txmanager

import "errors"

// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------

var (
	ErrNoDBAwareContext    = errors.New("no *gorm.DB found in context; ensure db injected using TxManagerHttpMiddleware or WithDB")
	ErrNoTransaction       = errors.New("transaction required, ensure you are in a transaction")
	ErrTransactionPresent  = errors.New("transaction present but propagation forbids it (PROPAGATION_NEVER)")
	ErrMultiplePropagation = errors.New("invalid propagation, multiple propagation has been provided")
)
