# Error handling in Go

## Why it exists
Go returns errors as ordinary values instead of throwing exceptions. The compiler
does not force you to look at them, so discipline is the only thing that stops a
failure from silently turning into wrong behavior later.

## The rule
Every error is either handled, returned with added context, or deliberately
ignored with a comment explaining why.

## Common mistakes
```go
data, _ := os.ReadFile(path)          // failure becomes an empty slice
defer f.Close()                       // fine for reads, but for writes Close can fail
if err != nil { return err }          // loses where it failed
```

## Better
```go
data, err := os.ReadFile(path)
if err != nil {
    return fmt.Errorf("read config %q: %w", path, err)
}
```
`%w` keeps the original error inspectable with `errors.Is` and `errors.As`.

## Try it yourself
Find a function in your PR that returns only `error`. Follow one call to it and
answer: what happens to the caller's state if it fails halfway?
