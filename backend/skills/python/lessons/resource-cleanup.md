# Close what you open, on every path

## Why it exists
Files, sockets, database connections and locks that are not released when an error happens
leak. A long-running service eventually runs out of file handles or deadlocks on a lock that
nobody frees.

## The rule
Use `with` for anything that needs closing. It releases the resource on success, on an
early `return` and when an exception is raised.

## Common mistakes
```python
f = open(path)
data = f.read()
f.close()      # skipped if read() raises
```

## Better
```python
with open(path) as f:
    data = f.read()
```

## Try it yourself
Add a `raise` just after the resource is opened. Does it still get closed?
