# gorm-txflow

**Easy-to-use Spring-style transaction manager for GORM.**  
Supports `REQUIRED`, `REQUIRES_NEW`, `NESTED`, `SUPPORTS`, `NOT_SUPPORTED`, `MANDATORY`, and `NEVER` propagation modes,
optional **isolation level** hints, read-only hints, and post-transaction hooks (e.g. `AfterCommit`). Transparent
middleware-friendly context injection (HTTP / Kafka / job runners). Includes a configurable, pool-backed `REQUIRES_NEW`
strategy for production correctness and test patterns for SQLite & Testcontainers.

[![Go Reference](https://img.shields.io/badge/go-reference-blue.svg)]() [![License](https://img.shields.io/badge/license-MIT-lightgrey.svg)]()

---

> **Goal:** Give Go/GORM apps Spring-like transactional semantics with minimal ceremony, safe defaults for performance,
> and an opt-in robust strategy for `NESTED` and `REQUIRES_NEW` propagations.

---

## Table of contents

- [Why gorm-txflow?](#why-gorm-txflow)
- [Features at a glance](#features-at-a-glance)
- [Quickstart](#quickstart)
- [Propagation modes (cheat sheet)](#propagation-modes-cheat-sheet)
- [Examples & patterns](#examples--patterns)
    - [REQUIRED (default)](#required-default)
    - [REQUIRES_NEW (production-safe)](#requires_new-production-safe)
    - [NESTED (savepoints)](#nested-savepoints)
    - [SUPPORTS / NOT_SUPPORTED / MANDATORY / NEVER](#supports--not_supported--mandatory--never)
- [Post-commit hooks (AfterCommit)](#post-commit-hooks-aftercommit)
- [Middleware & context wiring](#middleware--context-wiring)
- [Isolation levels & ReadOnly hints](#isolation-levels--readonly-hints)
- [Testing recommendations](#testing-recommendations)
- [Advanced: customizing REQUIRES_NEW strategy](#advanced-customizing-requires_new-strategy)
- [Errors, pitfalls & notes](#errors-pitfalls--notes)
- [API reference (essential)](#api-reference-essential)
- [Contribution & roadmap](#contribution--roadmap)
- [License](#license)

---

## Why gorm-txflow?

If you or your team ever used Spring’s `@Transactional` you know how helpful propagation modes and transaction
suspension are. `gorm-txflow` brings those semantics to GORM:

- declarative, composable transactions via a simple API
- post-commit hooks to reliably run side-effects only after commit
- middleware-friendly context propagation for HTTP, consumers, and jobs
- production-grade `REQUIRES_NEW` semantics via pooled connections (opt-in)
- test-friendly patterns (SQLite in-memory + Testcontainers for integration)

---

## Features at a glance

- Propagation modes: `REQUIRED` (default), `REQUIRES_NEW`, `NESTED`, `SUPPORTS`, `NOT_SUPPORTED`, `MANDATORY`, `NEVER`.
- Savepoint-based nesting for `NESTED`.
- Post-transaction hooks (AfterCommit) with panic capture and error aggregation.
- Middleware or programmatic `WithDB` to inject root DB into context.
- `GetDB` / `GetTx` / `MustGetDB` / `MustGetTx` helpers.
- Optional **pool-backed** `REQUIRES_NEW` that checks out a connection from `sql.DB` (not creating raw TCP every time).
- Test patterns: `file::memory:?cache=shared` for SQLite multi-connection testing; Testcontainers for MySQL/Postgres
  integration tests.
- Hooks execute only on successful commit.

---

## Quickstart

Install:

```bash
go get github.com/rancbar/gorm-txflow
```

---

## Basic usage

```go
rootCtx := txmanager.WithDB(context.Background(), db)

err := txmanager.DoInTransaction(rootCtx, func (ctx context.Context) error {
    txDB, _ := txmanager.GetDB(ctx)
    return txDB.Create(&User{Name: "alice"}).Error
})
```

---

## Propagation modes (cheat sheet)

| Mode            | Description                                                              |
|-----------------|--------------------------------------------------------------------------|
| `REQUIRED`      | Join existing tx if present; otherwise start new.                        |
| `REQUIRES_NEW`  | Suspend outer tx and run inner on a separate connection (opt-in pooled). |
| `NESTED`        | Use savepoints; rollback inner without affecting outer.                  |
| `SUPPORTS`      | Use tx if present; otherwise non-transactional.                          |
| `NOT_SUPPORTED` | Always non-transactional; suspends tx.                                   |
| `MANDATORY`     | Requires existing tx; errors if none.                                    |
| `NEVER`         | Errors if transaction exists.                                            |

---

## Examples & patterns

### REQUIRED (default)

```go
err := txflow.DoInTransaction(ctx, func (ctx context.Context) error {
    tx := txflow.MustGetDB(ctx)
    return tx.Create(&Item{Name: "x"}).Error
})
```

### REQUIRES_NEW (production-safe)

```go
doInTx := func (ctx context.Context) error {
    tx := txflow.MustGetDB(ctx)
    return tx.Create(&Item{Name: "inner"}).Error
}
err := DoInTransaction(ctx, doInTx, TxOptionWithPropagation(PropagationRequiresNew))
```

### NESTED (savepoints)

```go
err := txflow.DoInTransaction(ctx, func (ctx context.Context) error {
    tx := txflow.MustGetDB(ctx)
    err := tx.Create(&Item{Name: "outer"}).Error

    doInTx := func (ctx context.Context) error {
        tx2 := txflow.MustGetDB(ctx)
        err := tx2.Create(&Item{Name: "nested"}).Error
        return errors.New("fail nested") // Force Fail Transaction
    }
    err = txflow.DoInTransaction(ctx, doInTx, txflow.TxOptionWithPropagation(PropagationNested))

    return nil
})
```

### SUPPORTS / NOT_SUPPORTED / MANDATORY / NEVER

```go
txflow.DoInTransaction(ctx, fn, txflow.TxOptionWithPropagation(PropagationSupports))
txflow.DoInTransaction(ctx, fn, txflow.TxOptionWithPropagation(PropagationNotSupported))
txflow.DoInTransaction(ctx, fn, txflow.TxOptionWithPropagation(PropagationMandatory))
txflow.DoInTransaction(ctx, fn, txflow.TxOptionWithPropagation(PropagationNever))
```

### Post-commit hooks (AfterCommit)
```go
err := txflow.DoInTransaction(ctx, func(ctx context.Context) error {
    txflow.AfterCommit(func(ctx context.Context) error {
		
        return sendNotification(ctx, 'Done')
		
    })
    return nil
})
```
- Hooks run only after commit

- Panic-safe, aggregated errors

- Nested hooks attach to the outermost transaction

## Middleware & context wiring

### Inject DB into each request:

```go
http.Handle("/", TxManagerHttpMiddleware(db)(myHandler))
```

### Kafka / job runners:
```go
ctx := txmanager.WithDB(context.Background(), db)
```
`WithDB` does nothing if a DB already exists in the context (safe idempotent behavior).

---

## Isolation levels & ReadOnly hints
```go
opt := TxOptionWithPropagation(PropagationRequired).
            WithIsolationLevel(IsolationSerializable).
            WithReadOnly(true)
err := DoInTransaction(ctx, fn, opt)
```

Support depends on database/dialector.

---

## Testing recommendations

### Unit tests:
Use SQLite shared-cache in-memory:
```go
gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
```
Note: The SQLite itself is not able to support concurrent transactions and all features

### Integration tests:
Testcontainers for MySQL/Postgres to validate REQUIRES_NEW and real pooling behavior.

---

## Advanced: customizing REQUIRES_NEW strategy

```go
orig := BeginNewTransactionOnNewSession
BeginNewTransactionOnNewSession = beginNewTransactionWithDedicatedConn
defer func() { BeginNewTransactionOnNewSession = orig }()
```

Pooled strategy uses `sqlDB.Conn(ctx)` to guarantee independent commit/rollback.

---

## Errors, pitfalls & notes
- SQLite memory mode requires cache=shared for multi-connection tests.
- Default GORM NewDB does not guarantee connection isolation.
- Ensure DB pool allows enough connections for suspended + inner tx.
- Don’t run heavy tasks in hooks; enqueue jobs instead.

## API reference (essential)

### Usage APIs
```go
// Run in transaction
func DoInTransaction(ctx context.Context, fn func(ctx context.Context) error, txOptions ...TxOption) error

// Run after transaction commit
func AfterCommit(ctx context.Context, hook func(ctx context.Context) error)
```
You only need to use these functions after configuring the middlewares


### Middleware and DB injection approaches
```go
// HTTP middleware
func TxManagerHttpMiddleware(db *gorm.DB) func(http.Handler) http.Handler

// Manual Injecting
func WithDB(ctx context.Context, db *gorm.DB) context.Context
```

### Options methods
```go
// Create transaction options
func TxOptionWithPropagation(lvl PropagationLevel) TxOption
func TxOptionWithIsolationLevel(lvl sql.IsolationLevel) TxOption
func TxOptionWithReadonly(flag bool) TxOption
```

### Helper Methods
```go
// Helpers to access the db from context
func GetDB(ctx context.Context) (*gorm.DB, bool)
func MustGetDB(ctx context.Context) *gorm.DB
func GetTx(ctx context.Context) (*gorm.DB, bool)
func MustGetTx(ctx context.Context) *gorm.DB
```

---

## License
MIT License — see the [LICENSE](./LICENSE) file for full details.