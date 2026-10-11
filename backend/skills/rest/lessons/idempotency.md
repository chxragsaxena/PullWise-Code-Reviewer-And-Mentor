# Make repeatable requests safe to repeat

## Why it exists
Networks fail silently: a client that doesn't see a response often can't
tell whether the request succeeded before the connection dropped, and the
honest fix is retrying. If retrying a "charge the customer" request charges
them twice, the API has made an unreliable network into a user-facing bug.

## The rule
- `GET`, `PUT`, `DELETE` must be idempotent: calling them once or five times
  in a row leaves the system in the same state.
- `POST` is not idempotent by default. For a `POST` whose repetition is
  dangerous (payments, order creation), accept an idempotency key from the
  client and return the original result for a repeated key instead of
  repeating the effect.

## Common mistakes
```go
// PUT that increments instead of setting — not idempotent.
func addCredit(w http.ResponseWriter, r *http.Request) {
    account.Balance += parseAmount(r) // calling this twice adds it twice
}
```
```go
// POST /charge with no idempotency key: a retried request charges again.
func charge(w http.ResponseWriter, r *http.Request) {
    paymentProvider.Charge(parseAmount(r))
}
```

## Better
```go
func setBalance(w http.ResponseWriter, r *http.Request) {
    account.Balance = parseAmount(r) // same result no matter how many times
}
```
```go
func charge(w http.ResponseWriter, r *http.Request) {
    key := r.Header.Get("Idempotency-Key")
    if existing, ok := store.Lookup(key); ok {
        writeJSON(w, http.StatusOK, existing) // don't charge again
        return
    }
    result := paymentProvider.Charge(parseAmount(r))
    store.Save(key, result)
    writeJSON(w, http.StatusOK, result)
}
```

## Try it yourself
For a `POST` in your PR, ask: if the client's connection drops right after
the server processes this but before the response arrives, and the client
retries, what happens? If the answer is "it happens again," that's the risk
this check is about.
