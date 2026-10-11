# Give every promise somewhere to send its rejection

## Why it exists
A `.then()` chain that's never awaited, returned, or followed by `.catch()`
has no path for a rejection to go. In Node, an unhandled rejection can crash
the process; in a browser, it's a silent failure that shows up as "nothing
happened" with no error anyone sees.

## The rule
Every promise chain needs one of: a trailing `.catch(...)`, being `await`ed
inside a `try`/`catch`, or being returned to a caller that will handle it.

## Common mistakes
```ts
function save(order: Order) {
  api.post("/orders", order).then(() => toast("Saved"));
  // if the request fails, nothing happens — not even a log
}
```

## Better
```ts
function save(order: Order) {
  api.post("/orders", order)
    .then(() => toast("Saved"))
    .catch((err) => toast(`Failed to save: ${err.message}`));
}
```
or, with `async`/`await`:
```ts
async function save(order: Order) {
  try {
    await api.post("/orders", order);
    toast("Saved");
  } catch (err) {
    toast(`Failed to save: ${(err as Error).message}`);
  }
}
```

## Try it yourself
Find a `.then()` in your PR with no `.catch()`. If the request inside it
fails right now, what does the user see?
