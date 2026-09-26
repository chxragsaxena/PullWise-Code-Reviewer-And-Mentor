# Context propagation

## Why it exists
`context.Context` carries cancellation, deadlines and request-scoped values across
API boundaries. Without it, work continues after the caller has gone away: a
cancelled HTTP request still runs its database query.

## The rule
- Accept `ctx context.Context` as the first parameter of any function that does
  I/O or may block.
- Pass it down. Never store it in a struct or replace it with `context.Background()`
  mid-request.

## Common mistakes
```go
resp, err := http.Get(url)                       // no way to cancel or time out
db.Query("SELECT ...")                           // use QueryContext
func Fetch(id int, ctx context.Context) error    // ctx should come first
```

## Better
```go
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
rows, err := db.QueryContext(ctx, "SELECT ...")
func Fetch(ctx context.Context, id int) error
```

## Try it yourself
Cancel the request context in a test and check that your handler stops promptly.
