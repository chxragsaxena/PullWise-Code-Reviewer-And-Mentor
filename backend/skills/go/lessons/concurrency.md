# Concurrency: goroutine lifetimes

## Why it exists
Starting a goroutine is one keyword. Stopping it is your problem. A goroutine that
blocks forever on a channel is a leak: it holds memory and anything it references.

## The rule
Every goroutine you start needs an answer to two questions:
1. How does it stop? (context cancellation, closed channel, finite work)
2. Who waits for it? (`sync.WaitGroup`, `errgroup`, a result channel)

## Common mistakes
```go
go func() { for msg := range ch { handle(msg) } }()   // never stops if ch is never closed
go process(item)                                      // fire and forget, errors lost
```

## Better
```go
g, ctx := errgroup.WithContext(ctx)
g.Go(func() error { return process(ctx, item) })
if err := g.Wait(); err != nil { return err }
```

## Try it yourself
For each `go` statement in your PR, write one sentence saying when it exits.
If you can't, that is the finding.
