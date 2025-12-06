package txflow

import (
	"database/sql"
	"fmt"
)

type PropagationLevel int

const (
	PropagationDefault PropagationLevel = iota
	PropagationRequired
	PropagationRequiresNew
	PropagationSupports
	PropagationNotSupported
	PropagationMandatory
	PropagationNever
	PropagationNested
)

func (p PropagationLevel) String() string {
	switch p {
	case PropagationDefault:
		return "DEFAULT"
	case PropagationRequired:
		return "REQUIRED"
	case PropagationRequiresNew:
		return "REQUIRES_NEW"
	case PropagationSupports:
		return "SUPPORTS"
	case PropagationNotSupported:
		return "NOT_SUPPORTED"
	case PropagationMandatory:
		return "MANDATORY"
	case PropagationNever:
		return "NEVER"
	case PropagationNested:
		return "NESTED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(p))
	}
}

type TxOption struct {
	txOptions    sql.TxOptions
	setReadLevel bool
	Propagation  PropagationLevel
}

// WithPropagation Sets the propagation level and returns the tx option
func (t TxOption) WithPropagation(lvl PropagationLevel) TxOption {
	t.Propagation = lvl
	return t
}

// WithIsolationLevel Sets the isolation level and returns the tx option
func (t TxOption) WithIsolationLevel(lvl sql.IsolationLevel) TxOption {
	t.txOptions.Isolation = lvl
	return t
}

// WithReadOnly Sets the readonly flag and returns the tx option
func (t TxOption) WithReadOnly(flag bool) TxOption {
	t.txOptions.ReadOnly = flag
	t.setReadLevel = true
	return t
}

// TxOptionWithPropagation returns new transaction option with provided PropagationLevel
func TxOptionWithPropagation(lvl PropagationLevel) TxOption {
	return TxOption{}.WithPropagation(lvl)
}

// TxOptionWithIsolationLevel returns new transaction option with provided isolation level
func TxOptionWithIsolationLevel(lvl sql.IsolationLevel) TxOption {
	return TxOption{}.WithIsolationLevel(lvl)
}

// TxOptionWithReadonly returns new transaction option with provided read-only flag
func TxOptionWithReadonly(flag bool) TxOption {
	return TxOption{}.WithReadOnly(flag)
}

// mergeOptions returns a merged transaction option as result concluded from multiple provided options
func mergeOptions(options ...TxOption) (TxOption, error) {
	var result TxOption
	for _, option := range options {
		if option.Propagation != PropagationDefault {
			if result.Propagation != PropagationDefault && option.Propagation != result.Propagation {
				return result, ErrMultiplePropagation
			}
			result.Propagation = option.Propagation
		}
		if option.txOptions.Isolation != sql.LevelDefault {
			if result.txOptions.Isolation != sql.LevelDefault && option.txOptions.Isolation != result.txOptions.Isolation {
				return result, ErrMultipleIsolation
			}
			result.txOptions.Isolation = option.txOptions.Isolation
		}
		if option.setReadLevel == true {
			if result.setReadLevel && result.txOptions.ReadOnly != option.txOptions.ReadOnly {
				return result, ErrMultipleReadLevel
			}
			result.txOptions.ReadOnly = option.txOptions.ReadOnly
			result.setReadLevel = true
		}
	}
	return result, nil
}
