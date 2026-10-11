# Dependency Inversion Principle

## Why it exists
High-level logic (what the business rule is) shouldn't depend on low-level
detail (which database driver, which HTTP client). When it does, swapping
the detail — a different database for tests, a different provider in
production — means editing the business logic itself.

## The rule
Depend on an interface the high-level code defines, and let the low-level
implementation satisfy it — not the other way around. The abstraction
belongs to the consumer, not the library.

## Common mistakes
```go
type OrderService struct {
    db *postgres.Client // concrete, low-level: tied to one driver
}

func NewOrderService() *OrderService {
    return &OrderService{db: postgres.Connect(dsn)} // decides its own dependency
}
```
`OrderService` cannot be tested without a real Postgres connection, and
cannot be pointed at any other store without editing its source.

## Better
```go
// OrderStore is defined by the package that needs it, not by the database
// package — this is dependency inversion in practice, not just an interface.
type OrderStore interface {
    Save(ctx context.Context, o Order) error
}

type OrderService struct {
    db OrderStore
}

func NewOrderService(db OrderStore) *OrderService {
    return &OrderService{db: db} // the caller decides, and can pass a fake in tests
}
```
`*postgres.Client` now satisfies `OrderStore` implicitly; so can a fake, or a
different database, with zero changes to `OrderService`.

## Try it yourself
Find a type in your PR that constructs its own dependency (a client, a
store, a connection) instead of receiving it. Could that dependency instead
be a small interface, passed in by whoever creates this type?
