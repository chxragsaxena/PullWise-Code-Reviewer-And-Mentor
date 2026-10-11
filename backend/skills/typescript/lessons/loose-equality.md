# Strict equality, not loose equality

## Why it exists
`==` and `!=` convert both sides to a common type before comparing, using
rules that are easy to misremember: `0 == false`, `"" == 0`, `null ==
undefined`, but `null != 0`. `===`/`!==` compare the value as it actually
is, with no hidden conversion.

## The rule
Use `===` and `!==` by default. Reach for `==`/`!=` only for the one
deliberate case where you want `null`/`undefined` treated as equal to each
other and nothing else (`x == null`), and say so in a comment if you do.

## Common mistakes
```ts
if (count == "0") { ... }       // true even when count is the number 0
if (user.role == undefined) { ... } // also matches null, maybe not intended
```

## Better
```ts
if (count === "0") { ... }
if (user.role === undefined) { ... }
```

## Try it yourself
Find a `==` or `!=` in your PR. Write out what both sides could actually be
at runtime — if they're always the same type already, switching to strict
equality changes nothing except removing the coercion risk.
