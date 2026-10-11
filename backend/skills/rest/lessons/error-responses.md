# Use one error shape across the whole API

## Why it exists
A client that handles errors from one endpoint should be able to handle
errors from every endpoint the same way. If each handler invents its own
error JSON, every integration needs special-case code per endpoint, and
that code breaks the moment a new inconsistent shape ships.

## The rule
Pick one error shape for the whole API — for example
`{"error": {"code": "...", "message": "..."}}` — and use it everywhere,
including for framework-level errors (404, 405, panics), not just the ones
individual handlers produce on purpose.

## Common mistakes
```go
// Three handlers, three different shapes.
json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
json.NewEncoder(w).Encode(map[string]string{"message": "invalid input", "field": "email"})
json.NewEncoder(w).Encode(struct{ Err string `json:"err"` }{"server error"})
```
A client can't write one piece of code to extract "what went wrong" across
all three.

## Better
```go
type errorBody struct {
    Error struct {
        Code    string `json:"code"`
        Message string `json:"message"`
    } `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(errorBody{Error: struct {
        Code, Message string
    }{code, message}})
}
```
Every handler, and the router's own 404/405 responses, go through the same
function.

## Try it yourself
Find two error responses in your PR (or in the API it changes). Do they use
the same field names for "what went wrong"? If a client wrote one parser,
would it work for both?
