# Catch what you can handle, and never hide the rest

## Why it exists
`except:` catches everything, including `KeyboardInterrupt` and `SystemExit`, and an
`except` block that only says `pass` throws the error away. Either way the failure vanishes
at the place it happened and reappears later as a confusing bug with no trail.

## The rule
Catch the narrowest exception you actually know how to handle. If you cannot recover,
log it and let it continue upward. Ignoring an error should be a decision you can explain.

## Common mistakes
```python
try:
    save(order)
except:
    pass
```

## Better
```python
try:
    save(order)
except DatabaseError:
    logging.exception("could not save order %s", order.id)
    raise
```

## Try it yourself
Find an `except` in your PR. If it fired right now, would anyone find out?
