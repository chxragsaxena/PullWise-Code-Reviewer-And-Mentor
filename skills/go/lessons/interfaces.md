# Interfaces: small and defined by the consumer

## Why it exists
In Go, a type satisfies an interface implicitly. That means the package that
*uses* a behavior should declare the interface, listing only the methods it needs.

## The rule
- Prefer one to three methods.
- Define the interface where it is consumed, not next to the implementation.
- Don't introduce an interface until there is a second implementation or a test
  that needs a fake.

## Common mistakes
```go
type UserRepository interface {   // 12 methods, and every caller uses 1 or 2
    Get(...); List(...); Create(...); Update(...); Delete(...); // ...
}
```

## Better
```go
// in the package that sends the email
type userGetter interface {
    Get(ctx context.Context, id int64) (User, error)
}
```

## Try it yourself
Pick a function that takes a large interface. List the methods it actually calls.
Rewrite the parameter type as an interface containing only those.
