package txman

import (
	"fmt"
)

type Propagation int

const defaultPropagation = PropagationRequired
const (
	PropagationRequired Propagation = iota + 1
	PropagationRequiresNew
	PropagationSupports
	PropagationNotSupported
	PropagationMandatory
	PropagationNever
	PropagationNested
)

func (p Propagation) String() string {
	switch p {
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
	propagation *Propagation
}

// TxOptionWithPropagation returns new transaction option with provided propagation
func TxOptionWithPropagation(p Propagation) TxOption {
	return TxOption{propagation: &p}
}

// Propagation returns the propagation value (getter for unexported field)
func (o TxOption) Propagation() *Propagation { return o.propagation }

// fillOptionDefaults returns the option with defaults applied
func fillOptionDefaults(opt TxOption) TxOption {
	if opt.propagation == nil {
		dp := defaultPropagation
		opt.propagation = &dp
	}
	return opt
}

// mergeOptions returns a merged transaction option as result concluded from multiple provided options
func mergeOptions(options ...TxOption) (TxOption, error) {
	var result TxOption
	for _, option := range options {
		if option.propagation != nil {
			if result.propagation != nil && option.propagation != result.propagation {
				return TxOption{}, ErrMultiplePropagation
			}
			result.propagation = option.propagation
		}
	}
	result = fillOptionDefaults(result)
	return result, nil
}
