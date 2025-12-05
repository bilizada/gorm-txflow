package txmanager

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func Test_WithDB_AddsDBWhenAbsent_AndDoesNotOverwrite(t *testing.T) {
	base := context.Background()
	db1 := &gorm.DB{}
	ctx := WithDB(base, db1)

	// Should store db1
	got, ok := GetDB(ctx)
	if !ok || got != db1 {
		t.Fatalf("GetDB returned %v, %v; want db1", got, ok)
	}

	// Subsequent WithDB must NOT overwrite existing DB
	db2 := &gorm.DB{}
	ctx2 := WithDB(ctx, db2)
	got2, ok2 := GetDB(ctx2)
	if !ok2 || got2 != db1 {
		t.Fatalf("WithDB overwrote existing DB: got %p want %p", got2, db1)
	}
}

func Test_GetDB_PrefersTxOverRootDB(t *testing.T) {
	root := &gorm.DB{} // root DB
	tx := &gorm.DB{}   // tx DB
	ctx := context.WithValue(context.Background(), DbCtxKey{}, root)

	// When tx present, GetDB should return tx (and true)
	ctxWithTx := context.WithValue(ctx, TxDBKey{}, tx)
	got, ok := GetDB(ctxWithTx)
	if !ok || got != tx {
		t.Fatalf("GetDB did not prefer tx: got %v ok=%v; want tx", got, ok)
	}

	// If tx absent, GetDB should return root
	got2, ok2 := GetDB(ctx)
	if !ok2 || got2 != root {
		t.Fatalf("GetDB did not return root when tx absent: got %v ok=%v; want root", got2, ok2)
	}
}

func Test_MustGetDB_PanicsWhenMissing_ReturnsWhenPresent(t *testing.T) {
	// MustGetDB should panic when no DB present
	assertPanics(t, func() { MustGetDB(context.Background()) }, ErrNoDBAwareContext)

	// should return the DB when present (root)
	root := &gorm.DB{}
	ctx := WithDB(context.Background(), root)
	got := MustGetDB(ctx)
	if got != root {
		t.Fatalf("MustGetDB returned %v; want %v", got, root)
	}

	// when tx present MustGetDB should return tx
	tx := &gorm.DB{}
	ctxTx := context.WithValue(ctx, TxDBKey{}, tx)
	got2 := MustGetDB(ctxTx)
	if got2 != tx {
		t.Fatalf("MustGetDB did not prefer tx: got %v; want %v", got2, tx)
	}
}

func Test_GetTx_And_MustGetTx(t *testing.T) {
	// No tx -> GetTx false
	if tx, ok := GetTx(context.Background()); ok || tx != nil {
		t.Fatalf("GetTx returned %v, %v on empty ctx; want nil,false", tx, ok)
	}

	// With tx present
	tx := &gorm.DB{}
	ctx := context.WithValue(context.Background(), TxDBKey{}, tx)
	got, ok := GetTx(ctx)
	if !ok || got != tx {
		t.Fatalf("GetTx returned %v, %v; want tx,true", got, ok)
	}

	// MustGetTx returns tx
	if got2 := MustGetTx(ctx); got2 != tx {
		t.Fatalf("MustGetTx returned %v; want %v", got2, tx)
	}

	// MustGetTx panics when missing
	assertPanics(t, func() { MustGetTx(context.Background()) }, ErrNoTransaction)
}

// helper: assert that f panics with expected error (by equality)
func assertPanics(t *testing.T, f func(), expected error) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic with %v, but no panic occurred", expected)
		}
		// allow the panic to be either error or string wrapping error
		if err, ok := r.(error); ok {
			if err != expected {
				t.Fatalf("panic error = %v, want %v", err, expected)
			}
			return
		}
		// otherwise format and compare string
		if s, ok := r.(string); ok {
			if s != expected.Error() {
				t.Fatalf("panic string = %q, want %q", s, expected.Error())
			}
			return
		}
		t.Fatalf("panic value = %#v, want %v", r, expected)
	}()
	f()
}
