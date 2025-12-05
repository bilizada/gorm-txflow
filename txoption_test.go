package txmanager

import (
	"testing"
)

func TestPropagationString(t *testing.T) {
	cases := []struct {
		in  Propagation
		out string
	}{
		{PropagationRequired, "REQUIRED"},
		{PropagationRequiresNew, "REQUIRES_NEW"},
		{PropagationSupports, "SUPPORTS"},
		{PropagationNotSupported, "NOT_SUPPORTED"},
		{PropagationMandatory, "MANDATORY"},
		{PropagationNever, "NEVER"},
		{PropagationNested, "NESTED"},
		{Propagation(999), "UNKNOWN(999)"},
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
	if opt.propagation == nil {
		t.Fatalf("TxOptionWithPropagation produced nil propagation")
	}
	if got := opt.Propagation(); got == nil || *got != p {
		t.Fatalf("Propagation getter returned %v, want pointer to %v", got, p)
	}
}

func TestFillOptionDefaults(t *testing.T) {
	// when propagation is nil -> default applied
	var opt TxOption
	res := fillOptionDefaults(opt)
	if res.propagation == nil {
		t.Fatalf("fillOptionDefaults did not set default propagation")
	}
	if *res.propagation != defaultPropagation {
		t.Fatalf("fillOptionDefaults set %v, want default %v", *res.propagation, defaultPropagation)
	}

	// when propagation is set -> preserved
	p := PropagationNever
	opt2 := TxOption{propagation: &p}
	res2 := fillOptionDefaults(opt2)
	if res2.propagation == nil || *res2.propagation != p {
		t.Fatalf("fillOptionDefaults changed provided propagation, got %v want %v", res2.propagation, &p)
	}
}

func TestMergeOptions(t *testing.T) {
	t.Run("no options -> default", func(t *testing.T) {
		res, err := mergeOptions()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.propagation == nil || *res.propagation != defaultPropagation {
			t.Fatalf("mergeOptions() = %v, want default propagation %v", res.propagation, defaultPropagation)
		}
	})

	t.Run("single option preserved", func(t *testing.T) {
		o := TxOptionWithPropagation(PropagationRequiresNew)
		res, err := mergeOptions(o)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.propagation == nil || *res.propagation != PropagationRequiresNew {
			t.Fatalf("mergeOptions single = %v, want %v", res.propagation, PropagationRequiresNew)
		}
	})

	t.Run("same pointer merged", func(t *testing.T) {
		p := PropagationMandatory
		o1 := TxOption{propagation: &p}
		o2 := TxOption{propagation: &p}
		res, err := mergeOptions(o1, o2)
		if err != nil {
			t.Fatalf("unexpected error merging same pointer: %v", err)
		}
		if res.propagation == nil || *res.propagation != PropagationMandatory {
			t.Fatalf("mergeOptions same pointer = %v, want %v", res.propagation, PropagationMandatory)
		}
	})

	t.Run("same value but different pointers -> ErrMultiplePropagation", func(t *testing.T) {
		o1 := TxOptionWithPropagation(PropagationRequired)
		o2 := TxOptionWithPropagation(PropagationRequired)
		_, err := mergeOptions(o1, o2)
		if err == nil {
			t.Fatalf("expected error when merging same value with different pointers, got nil")
		}
		// prefer checking the error name if available in package
		if err != ErrMultiplePropagation {
			t.Fatalf("expected ErrMultiplePropagation, got %v", err)
		}
	})

	t.Run("conflicting different values -> ErrMultiplePropagation", func(t *testing.T) {
		o1 := TxOptionWithPropagation(PropagationRequired)
		o2 := TxOptionWithPropagation(PropagationNever)
		_, err := mergeOptions(o1, o2)
		if err == nil {
			t.Fatalf("expected error when merging conflicting propagations, got nil")
		}
		if err != ErrMultiplePropagation {
			t.Fatalf("expected ErrMultiplePropagation, got %v", err)
		}
	})

	t.Run("one option nil and one set -> preserved", func(t *testing.T) {
		o1 := TxOption{} // nil propagation
		o2 := TxOptionWithPropagation(PropagationSupports)
		res, err := mergeOptions(o1, o2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.propagation == nil || *res.propagation != PropagationSupports {
			t.Fatalf("mergeOptions result = %v, want %v", res.propagation, PropagationSupports)
		}
	})
}
