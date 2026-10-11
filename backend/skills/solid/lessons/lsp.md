# Liskov Substitution Principle

## Why it exists
Anywhere a base type or interface is expected, a subtype should be usable
without the caller needing to know which one it got. If a subtype secretly
does less, or demands more, than the contract promises, every caller that
relies on the contract is at risk — not just the ones that happen to test
against that particular subtype.

## The rule
A subtype must honor its base type's contract: it can't demand stricter
inputs (narrowed preconditions) than the base allows, and it can't deliver a
weaker guarantee (widened postconditions, new failure modes) than the base
promises.

## Common mistakes
```go
type Writer interface {
    Write(p []byte) (int, error)
}

type ReadOnlyFile struct{ /* ... */ }

func (f *ReadOnlyFile) Write(p []byte) (int, error) {
    panic("ReadOnlyFile does not support Write") // breaks the Writer contract
}
```
Anything that calls `Write` through a `Writer` has no reason to expect a
panic — the interface promises an `error`, not a crash. Code that works with
every other `Writer` breaks specifically for this one.

## Better
Don't implement an interface you can't actually honor.
```go
// ReadOnlyFile simply isn't a Writer. Callers that need to write require a
// Writer explicitly, so a ReadOnlyFile is never accepted where one is needed.
type ReadOnlyFile struct{ /* ... */ }
```
If read-only and read-write cases must share some code, share it through a
smaller interface both can honestly satisfy (see Interface Segregation),
rather than one or the other faking support it doesn't have.

## Try it yourself
For a type implementing an interface in your PR, check every method: does
it accept everything the interface's documented contract allows, and does
it only fail the ways the contract says it can?
