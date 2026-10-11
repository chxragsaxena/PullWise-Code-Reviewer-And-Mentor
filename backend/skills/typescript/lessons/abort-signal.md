# Make network requests cancellable

## Why it exists
A `fetch()` with no `AbortSignal` keeps running on the network even after
the caller has stopped caring — the user navigated away, a newer request
superseded it, or a timeout should have ended it. The response, once it
arrives, is wasted work, and in the worst case (a stale response arriving
after a newer one) it can overwrite newer data with older data.

## The rule
Pass an `AbortController`'s signal to any `fetch()` whose caller might stop
caring before it resolves — which, in interactive UIs, is most of them.

## Common mistakes
```ts
async function loadUser(id: string) {
  const res = await fetch(`/api/users/${id}`); // can't be cancelled
  return res.json();
}
```
If the user navigates to a different page while this is in flight, it keeps
running anyway.

## Better
```ts
async function loadUser(id: string, signal: AbortSignal) {
  const res = await fetch(`/api/users/${id}`, { signal });
  return res.json();
}

// at the call site:
const controller = new AbortController();
loadUser(id, controller.signal);
// later, e.g. on unmount:
controller.abort();
```

## Try it yourself
Find a `fetch()` in your PR with no second argument. If the component or
function that called it goes away before the response arrives, does
anything stop that request?
