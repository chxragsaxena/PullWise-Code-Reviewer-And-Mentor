# Return the status code that matches what actually happened

## Why it exists
Clients branch on status codes, not response bodies, to decide whether to
retry, give up, prompt the user, or redirect to login. A `200` on failure
means a client sees "success" and keeps going with bad data; a generic
`500` on a validation error means a client retries something that will
never succeed.

## The rule
- `200`/`201`/`204` — success; `201` for a create that returns the new
  resource, `204` for a success with no body.
- `400` — the request itself was invalid (bad input, failed validation).
- `401`/`403` — not authenticated / not allowed.
- `404` — the resource doesn't exist.
- `409` — conflicts with the current state (e.g. a duplicate, a version mismatch).
- `429` — rate limited; include `Retry-After` where possible.
- `500` — the server's own fault, not the caller's.

## Common mistakes
```go
func handler(w http.ResponseWriter, r *http.Request) {
    user, err := lookupUser(r)
    if err != nil {
        w.WriteHeader(http.StatusOK) // always 200, even on failure
        json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    ...
}
```
A client that only checks the status code sees success and renders an
"error" string as if it were a user.

## Better
```go
user, err := lookupUser(r)
switch {
case errors.Is(err, ErrNotFound):
    writeError(w, http.StatusNotFound, "user_not_found", err.Error())
    return
case err != nil:
    writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
    return
}
```

## Try it yourself
For an error path in your PR, check: does the status code tell a client
*what kind* of failure this was (their mistake vs. ours vs. not found),
or is it a `200`/generic `500` regardless of cause?
