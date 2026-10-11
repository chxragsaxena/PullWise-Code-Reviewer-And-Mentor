# Don't leave console.log in

## Why it exists
`console.log` is almost always a debugging aid written to answer a question
while the code was being written. Left in, it clutters production logs (or a
user's browser console) with information that has nothing to do with
running the application correctly.

## The rule
Remove debug `console.log` calls before merging. If the output genuinely
needs to persist, use a real logger (with a level, so it can be filtered)
instead.

## Common mistakes
```ts
function total(items: Item[]) {
  console.log("items", items); // left over from debugging
  return items.reduce((sum, i) => sum + i.price, 0);
}
```

## Better
```ts
function total(items: Item[]) {
  return items.reduce((sum, i) => sum + i.price, 0);
}
// or, if it needs to stay:
logger.debug("computing total", { itemCount: items.length });
```

## Try it yourself
For a `console.log` in your PR, ask: would you want this line to run every
time this code executes in production? If not, it was debugging output.
