# Never build SQL by formatting strings

## Why it exists
When a query is assembled with f-strings, `%` or `+`, a value that contains SQL becomes part
of the command. A name like `x'; DROP TABLE users; --` then runs as code instead of being
stored as data.

## The rule
Write the query with placeholders and pass the values separately. The database driver sends
them as data and quotes them correctly.

## Common mistakes
```python
cur.execute(f"SELECT * FROM users WHERE name = '{name}'")
```

## Better
```python
cur.execute("SELECT * FROM users WHERE name = %s", (name,))
```
Table and column names cannot be placeholders; pick them from a fixed allow-list.

## Try it yourself
Search your PR for `execute(`. Does any argument contain `f"`, `%` or `+` with user input?
