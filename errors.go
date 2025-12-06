package txflow

import "errors"

// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------

var (
	ErrNoDBAwareContext    = errors.New("no *gorm.DB found in context; ensure db injected using TxManagerHttpMiddleware or WithDB")
	ErrNoTransaction       = errors.New("transaction required, ensure you are in a transaction")
	ErrTransactionPresent  = errors.New("transaction present but propagation forbids it (PROPAGATION_NEVER)")
	ErrMultiplePropagation = errors.New("invalid propagation level, multiple different propagation level has been provided")
	ErrMultipleReadLevel   = errors.New("invalid readonly level, multiple different readonly level has been provided")
	ErrMultipleIsolation   = errors.New("invalid isolation level, multiple different isolation level has been provided")
	ErrInvalidPropagation  = errors.New("invalid propagation")
)
