package txflow

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"
)

func Test_TxManagerHttpMiddleware_WithNilDB_PassesOriginalContext(t *testing.T) {
	mw := TxManagerHttpMiddleware(nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// ensure no DB was attached to context
		if v := r.Context().Value(DbCtxKey{}); v != nil {
			t.Fatalf("expected no DB in context, got %T %#v", v, v)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("next handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
	}
}

func Test_TxManagerHttpMiddleware_WithDB_AttachesDBToContext(t *testing.T) {
	// use a dummy *gorm.DB pointer (we do not call any methods on it)
	db := &gorm.DB{}

	mw := TxManagerHttpMiddleware(db)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// ensure DB was attached to context and is the same pointer
		v := r.Context().Value(DbCtxKey{})
		if v == nil {
			t.Fatalf("expected DB in context, got nil")
		}
		got, ok := v.(*gorm.DB)
		if !ok {
			t.Fatalf("expected *gorm.DB in context, got %T", v)
		}
		if got != db {
			t.Fatalf("context DB pointer mismatch: got %p want %p", got, db)
		}
		w.WriteHeader(http.StatusAccepted)
	})

	handler := mw(next)

	req := httptest.NewRequest(http.MethodPost, "http://example.test/do", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("next handler was not called")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusAccepted)
	}
}
