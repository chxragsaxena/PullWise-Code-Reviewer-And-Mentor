# Interface Segregation Principle

## Why it exists
A consumer shouldn't have to depend on methods it never calls. A large
interface couples every implementation and every test double to the whole
thing, even when a caller only ever uses one or two of its methods.

## The rule
- Prefer one to three methods per interface.
- Define the interface where it is consumed, not next to the implementation
  (idiomatic in Go: accept small interfaces, return concrete types).
- Don't introduce an interface until there's a second implementation, or a
  test that needs a fake.

## Common mistakes
```go
type UserRepository interface {
    Get(ctx context.Context, id int64) (User, error)
    List(ctx context.Context) ([]User, error)
    Create(ctx context.Context, u User) error
    Update(ctx context.Context, u User) error
    Delete(ctx context.Context, id int64) error
    Count(ctx context.Context) (int, error)
}

// in the package that sends a welcome email
func SendWelcome(repo UserRepository, id int64) error {
    user, err := repo.Get(context.Background(), id) // only ever calls Get
    ...
}
```
`SendWelcome` depends on a 6-method interface to use one method. Every fake
written for this function's tests has to stub five methods it never calls.

## Better
```go
// in the package that sends the email
type userGetter interface {
    Get(ctx context.Context, id int64) (User, error)
}

func SendWelcome(repo userGetter, id int64) error {
    user, err := repo.Get(context.Background(), id)
    ...
}
```
Anything with a `Get` method satisfies this — the full repository included —
and a test fake needs exactly one method.

## Try it yourself
Pick a function that takes a large interface. List the methods it actually
calls, then rewrite the parameter type as an interface containing only
those.
