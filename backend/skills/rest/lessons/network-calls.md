# Don't make a network or database call the request doesn't need

## Why it exists
Every network or database round trip adds latency the caller pays for and a
new way for the request to fail. A handler that makes the same call twice,
or calls a service for data it already has, is slower and less reliable
than it needs to be — and the cost compounds under load.

## The rule
- Fetch data once per request; pass it down instead of re-fetching.
- Watch for N+1 patterns: one query to list items, then one more query per
  item in a loop.
- Combine calls that always happen together into one, where the dependency
  being called supports it.

## Common mistakes
```go
func listOrders(w http.ResponseWriter, r *http.Request) {
    orders := db.ListOrders(ctx)
    for i, o := range orders {
        // one extra query per order — N+1
        orders[i].Customer = db.GetCustomer(ctx, o.CustomerID)
    }
    writeJSON(w, http.StatusOK, orders)
}
```
Ten orders means eleven queries for data a single join or batch call could
return in one or two.

## Better
```go
func listOrders(w http.ResponseWriter, r *http.Request) {
    orders := db.ListOrdersWithCustomers(ctx) // one query, joined
    writeJSON(w, http.StatusOK, orders)
}
```
Or, if the dependency doesn't support a join: collect the IDs first, then
make one batch call (`GetCustomers(ctx, ids)`) instead of one call per row.

## Try it yourself
Find a loop in your PR that calls a database or external service inside it.
Could the data for every iteration be fetched once, before the loop, in a
single call?
