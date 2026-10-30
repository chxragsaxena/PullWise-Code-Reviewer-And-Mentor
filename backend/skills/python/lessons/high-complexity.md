# Keep functions small enough to hold in your head

## Why it exists
Cyclomatic complexity counts the independent paths through a function: every `if`, loop,
`except`, `and`/`or` and conditional expression adds one. Past about 15, a function has more
paths than anyone tests, and bugs live in the paths nobody runs.

## The rule
When a function grows many branches, give each decision a name by moving it into its own
function.

## Common mistakes
One function that validates input, branches on the type, loops over results and handles each
failure inline.

## Better
```python
def handle(order):
    validate(order)
    price = quote(order)
    return charge(order, price)
```
Each helper has a few paths of its own and can be tested alone.

## Try it yourself
Pick the longest function in your PR. Can you describe what it does in one sentence without
saying "and"?
