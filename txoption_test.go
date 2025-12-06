package txman

import (
	"errors"
	"testing"
)

func TestPropagationString(t *testing.T) {
	cases := []struct {
		in  PropagationLevel
		out string
	}{
		{PropagationDefault, "DEFAULT"},
		{PropagationRequired, "REQUIRED"},
		{PropagationRequiresNew, "REQUIRES_NEW"},
		{PropagationSupports, "SUPPORTS"},
		{PropagationNotSupported, "NOT_SUPPORTED"},
		{PropagationMandatory, "MANDATORY"},
		{PropagationNever, "NEVER"},
		{PropagationNested, "NESTED"},
		{PropagationLevel(999), "UNKNOWN(999)"},
	}

	for _, c := range cases {
		if s := c.in.String(); s != c.out {
			t.Fatalf("Propagation(%d).String() = %q, want %q", int(c.in), s, c.out)
		}
	}
}

func TestTxOptionWithPropagationAndGetter(t *testing.T) {
	p := PropagationSupports
	opt := TxOptionWithPropagation(p)
	if got := opt.Propagation; got != p {
		t.Fatalf("Propagation getter returned %v, want pointer to %v", got, p)
	}
}

func TestMergeOptions(t *testing.T) {

	t.Run("single option preserved", func(t *testing.T) {
		o := TxOptionWithPropagation(PropagationRequiresNew)
		res, err := mergeOptions(o)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Propagation != PropagationRequiresNew {
			t.Fatalf("mergeOptions single = %v, want %v", res.Propagation, PropagationRequiresNew)
		}
	})

	t.Run("same pointer merged", func(t *testing.T) {
		p := PropagationMandatory
		o1 := TxOption{Propagation: p}
		o2 := TxOption{Propagation: p}
		res, err := mergeOptions(o1, o2)
		if err != nil {
			t.Fatalf("unexpected error merging same pointer: %v", err)
		}
		if res.Propagation != PropagationMandatory {
			t.Fatalf("mergeOptions same pointer = %v, want %v", res.Propagation, PropagationMandatory)
		}
	})

	t.Run("same value but different pointers -> ErrMultiplePropagation", func(t *testing.T) {
		o1 := TxOptionWithPropagation(PropagationRequired)
		o2 := TxOptionWithPropagation(PropagationRequired)
		_, err := mergeOptions(o1, o2)
		// prefer checking the error name if available in package
		if errors.Is(err, ErrMultiplePropagation) {
			t.Fatalf("expected no ErrMultiplePropagation error for same propagations, got %v", err)
		}
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
	})

	t.Run("conflicting different values -> ErrMultiplePropagation", func(t *testing.T) {
		o1 := TxOptionWithPropagation(PropagationRequired)
		o2 := TxOptionWithPropagation(PropagationNever)
		_, err := mergeOptions(o1, o2)
		if err == nil {
			t.Fatalf("expected error when merging conflicting propagations, got nil")
		}
		if !errors.Is(err, ErrMultiplePropagation) {
			t.Fatalf("expected ErrMultiplePropagation, got %v", err)
		}
	})

	t.Run("one option nil and one set -> preserved", func(t *testing.T) {
		o1 := TxOption{} // PropagationDefault
		o2 := TxOptionWithPropagation(PropagationSupports)
		res, err := mergeOptions(o1, o2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Propagation != PropagationSupports {
			t.Fatalf("mergeOptions result = %v, want %v", res.Propagation, PropagationSupports)
		}
	})
}
