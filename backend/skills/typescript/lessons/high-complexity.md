# Keep functions simple enough to hold in your head

## Why it exists
Cyclomatic complexity counts the independent paths through a function: each
`if`, loop, `switch` case and `&&`/`||` adds one. The more paths there are,
the more combinations of them exist, and the harder it is to be sure every
combination was actually tested.

## The rule
When a function's complexity crosses the threshold, split it: pull
validation, branching, or one case's logic into its own named function.

## Common mistakes
```ts
function handle(req: Request): Response {
  if (!req.user) { ... }
  if (req.method === "GET") { ... }
  else if (req.method === "POST") {
    if (req.body.type === "a") { ... }
    else if (req.body.type === "b") { ... }
    // ...and it keeps growing
  }
}
```

## Better
```ts
function handle(req: Request): Response {
  if (!req.user) return unauthorized();
  return routeByMethod(req);
}

function routeByMethod(req: Request): Response {
  if (req.method === "GET") return handleGet(req);
  if (req.method === "POST") return handlePost(req);
  return methodNotAllowed();
}
```
Each piece is now small enough to read, test and reason about on its own.

## Try it yourself
Count the `if`/`for`/`while`/`case`/`&&`/`||` in the longest function in
your PR. If someone added it up without you, would the number surprise you?
