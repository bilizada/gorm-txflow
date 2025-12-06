package txman

import (
	"database/sql"
	"errors"
	"testing"
)

func TestPropagationString(t *testing.T) {
	cases := []struct {
		name string
		in   PropagationLevel
		out  string
	}{
		{"default", PropagationDefault, "DEFAULT"},
		{"required", PropagationRequired, "REQUIRED"},
		{"requires_new", PropagationRequiresNew, "REQUIRES_NEW"},
		{"supports", PropagationSupports, "SUPPORTS"},
		{"not_supported", PropagationNotSupported, "NOT_SUPPORTED"},
		{"mandatory", PropagationMandatory, "MANDATORY"},
		{"never", PropagationNever, "NEVER"},
		{"nested", PropagationNested, "NESTED"},
		{"unknown", PropagationLevel(999), "UNKNOWN(999)"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if s := c.in.String(); s != c.out {
				t.Fatalf("got %q want %q", s, c.out)
			}
		})
	}
}

func TestTxOptionConstructors(t *testing.T) {
	t.Run("TxOptionWithPropagation", func(t *testing.T) {
		p := TxOptionWithPropagation(PropagationRequiresNew)
		if p.Propagation != PropagationRequiresNew {
			t.Fatalf("got %v want %v", p.Propagation, PropagationRequiresNew)
		}
	})

	t.Run("TxOptionWithIsolationLevel", func(t *testing.T) {
		iso := TxOptionWithIsolationLevel(sql.LevelSerializable)
		if iso.txOptions.Isolation != sql.LevelSerializable {
			t.Fatalf("got %v want %v", iso.txOptions.Isolation, sql.LevelSerializable)
		}
	})

	t.Run("TxOptionWithReadonly", func(t *testing.T) {
		ro := TxOptionWithReadonly(true)
		if ro.txOptions.ReadOnly != true || !ro.setReadLevel {
			t.Fatalf("got %+v; want ReadOnly=true and setReadLevel=true", ro)
		}
	})
}

func TestMergeOptions_DefaultsAndNonConflicting(t *testing.T) {
	t.Run("no options yields defaults", func(t *testing.T) {
		res, err := mergeOptions()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Propagation != PropagationDefault {
			t.Fatalf("expected default propagation; got %v", res.Propagation)
		}
		if res.txOptions.Isolation != sql.LevelDefault {
			t.Fatalf("expected default isolation; got %v", res.txOptions.Isolation)
		}
		if res.setReadLevel {
			t.Fatalf("expected setReadLevel=false; got true")
		}
	})

	t.Run("non-conflicting combine", func(t *testing.T) {
		o1 := TxOptionWithPropagation(PropagationSupports)
		o2 := TxOptionWithIsolationLevel(sql.LevelReadCommitted)
		o3 := TxOptionWithReadonly(false)

		res, err := mergeOptions(o1, o2, o3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Propagation != PropagationSupports {
			t.Fatalf("expected PropagationSupports; got %v", res.Propagation)
		}
		if res.txOptions.Isolation != sql.LevelReadCommitted {
			t.Fatalf("expected Isolation LevelReadCommitted; got %v", res.txOptions.Isolation)
		}
		if !res.setReadLevel || res.txOptions.ReadOnly != false {
			t.Fatalf("expected ReadOnly=false and setReadLevel=true; got %+v", res)
		}
	})
}

func TestMergeOptions_Conflicts(t *testing.T) {
	t.Run("conflicting propagation", func(t *testing.T) {
		o1 := TxOptionWithPropagation(PropagationRequired)
		o2 := TxOptionWithPropagation(PropagationNever)

		_, err := mergeOptions(o1, o2)
		if !errors.Is(err, ErrMultiplePropagation) {
			t.Fatalf("expected ErrMultiplePropagation, got %v", err)
		}
	})

	t.Run("conflicting isolation", func(t *testing.T) {
		o1 := TxOptionWithIsolationLevel(sql.LevelReadCommitted)
		o2 := TxOptionWithIsolationLevel(sql.LevelSerializable)

		_, err := mergeOptions(o1, o2)
		if !errors.Is(err, ErrMultipleIsolation) {
			t.Fatalf("expected ErrMultipleIsolation, got %v", err)
		}
	})

	t.Run("conflicting readonly", func(t *testing.T) {
		o1 := TxOptionWithReadonly(true)
		o2 := TxOptionWithReadonly(false)

		_, err := mergeOptions(o1, o2)
		if !errors.Is(err, ErrMultipleReadLevel) {
			t.Fatalf("expected ErrMultipleReadLevel, got %v", err)
		}
	})
}

func TestMergeOptions_SameValuesNoError(t *testing.T) {
	t.Run("same propagation repeated", func(t *testing.T) {
		o1 := TxOptionWithPropagation(PropagationRequired)
		o2 := TxOptionWithPropagation(PropagationRequired)
		res, err := mergeOptions(o1, o2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Propagation != PropagationRequired {
			t.Fatalf("expected PropagationRequired; got %v", res.Propagation)
		}
	})

	t.Run("same isolation repeated", func(t *testing.T) {
		i1 := TxOptionWithIsolationLevel(sql.LevelRepeatableRead)
		i2 := TxOptionWithIsolationLevel(sql.LevelRepeatableRead)
		res, err := mergeOptions(i1, i2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.txOptions.Isolation != sql.LevelRepeatableRead {
			t.Fatalf("expected LevelRepeatableRead; got %v", res.txOptions.Isolation)
		}
	})

	t.Run("same readonly repeated", func(t *testing.T) {
		r1 := TxOptionWithReadonly(true)
		r2 := TxOptionWithReadonly(true)
		res, err := mergeOptions(r1, r2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !res.setReadLevel || res.txOptions.ReadOnly != true {
			t.Fatalf("expected ReadOnly=true and setReadLevel=true; got %+v", res)
		}
	})
}
