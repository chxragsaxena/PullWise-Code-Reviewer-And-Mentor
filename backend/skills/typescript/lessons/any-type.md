# Avoid `any`

## Why it exists
`any` isn't "unknown type" — it's "stop type-checking this." Once a value is
`any`, every property access, method call and assignment through it skips
the compiler entirely, including ones that would have been caught as typos
or type mismatches anywhere else in the file.

## The rule
Give the value its real type. If the type genuinely isn't known at this
point (e.g. parsing unknown JSON), use `unknown` instead — it still forces a
check (a type guard, an assertion) before the value can be used.

## Common mistakes
```ts
function handle(payload: any) {
  return payload.user.name.toUpperCase(); // no error, even if any of this is wrong
}
```

## Better
```ts
function handle(payload: unknown) {
  if (isUserPayload(payload)) {
    return payload.user.name.toUpperCase(); // now checked
  }
  throw new Error("unexpected payload shape");
}
```

## Try it yourself
Find an `any` in your PR. If it were `unknown` instead, what would the
compiler force you to check before the code compiles? That check is exactly
what `any` is currently skipping.
