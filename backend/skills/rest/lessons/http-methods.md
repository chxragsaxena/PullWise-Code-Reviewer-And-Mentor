# Use the HTTP method that matches what the handler does

## Why it exists
HTTP methods carry meaning that clients, browsers, caches and proxies all
rely on: `GET` is safe to repeat, cache and prefetch; `POST`/`PUT`/`DELETE`
are not. A handler that mutates state behind a `GET`, or that reads data
behind a `POST` with no real reason, breaks those assumptions for everything
in front of it.

## The rule
- `GET` — read-only. Never changes state. Safe to retry, cache and prefetch.
- `POST` — create something, or an action with no other method that fits.
- `PUT` — replace a resource entirely, idempotently (see the idempotency lesson).
- `PATCH` — apply a partial update.
- `DELETE` — remove a resource, idempotently.

## Common mistakes
```go
// A GET that deletes — a crawler, prefetcher, or retried request can wipe data.
mux.HandleFunc("GET /users/{id}/delete", deleteUser)
```
```go
// A POST used for a pure lookup that could be a GET with query params.
mux.HandleFunc("POST /users/search", searchUsers)
```

## Better
```go
mux.HandleFunc("DELETE /users/{id}", deleteUser)
mux.HandleFunc("GET /users?q={query}", searchUsers)
```

## Try it yourself
For a handler in your PR, ask: if this request were retried automatically
by the network layer, or prefetched by a browser, would that be safe? If
not, it shouldn't be a `GET`.
